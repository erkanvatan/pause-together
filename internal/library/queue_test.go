package library

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/erkanvatan/pause-together/internal/media"
)

// runScans starts the queue's worker until the test ends.
func runScans(t *testing.T, q *Scans) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		q.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

// waitIdle waits until nothing is queued or scanning.
func waitIdle(t *testing.T, q *Scans) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		busy := false
		for _, st := range q.Status() {
			busy = busy || st.State != ""
		}
		if !busy {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("scans still busy: %+v", q.Status())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A scan that is running when its library is removed must not bring the videos back.
func TestScansRemoveDuringScan(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	l.write("Ronin (1998).mkv", "ronin")
	l.scan()
	l.write("Heat (1995).mkv", "heat, changed") // so the next scan probes it

	started, release := make(chan struct{}), make(chan struct{})
	l.scanner.Prober = proberFunc(func(context.Context, string) (media.Info, error) {
		close(started)
		<-release
		return fakeInfo, nil
	})
	q := NewScans(l.scanner)
	runScans(t, q)
	q.Request(l.lib.ID)
	<-started

	want := map[int64]ScanStatus{l.lib.ID: {State: ScanScanning, Done: 0, Total: 2}}
	if got := q.Status(); !reflect.DeepEqual(got, want) {
		t.Errorf("status mid-scan = %+v, want %+v", got, want)
	}
	if err := (&Libraries{DB: l.db, Root: l.scanner.Root}).Remove(context.Background(), l.lib.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitIdle(t, q)

	for p, v := range l.videos() {
		if !v.Missing {
			t.Errorf("%s: %+v, want missing", p, v)
		}
	}
	if got := q.Status(); len(got) != 0 {
		t.Errorf("status after = %+v, want empty: a removed library is no error", got)
	}
}

// Removed stops the running scan, so it writes nothing once the library is re-added: only a fresh
// scan, with the new type, may.
func TestScansRemoveAndReAddDuringScan(t *testing.T) {
	l := newTestLib(t, Movies)
	libs := &Libraries{DB: l.db, Root: l.scanner.Root}
	ctx := context.Background()
	l.write("Heat (1995).mkv", "heat")
	l.write("Ronin (1998).mkv", "ronin")
	l.scan()
	l.write("Heat (1995).mkv", "heat, changed") // so the next scan probes it

	started, release := make(chan struct{}), make(chan struct{})
	l.scanner.Prober = proberFunc(func(context.Context, string) (media.Info, error) {
		close(started)
		<-release
		return fakeInfo, nil
	})
	q := NewScans(l.scanner)
	runScans(t, q)
	q.Request(l.lib.ID)
	<-started

	if err := libs.Remove(ctx, l.lib.ID); err != nil {
		t.Fatal(err)
	}
	q.Removed(l.lib.ID)
	if _, err := libs.Add(ctx, l.lib.Path, OtherVideos); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitIdle(t, q)

	for p, v := range l.videos() {
		if !v.Missing {
			t.Errorf("%s: %+v, want still missing: the old scan must not write", p, v)
		}
	}
	if got := q.Status(); len(got) != 0 {
		t.Errorf("status after = %+v, want empty", got)
	}
}

// A removed library's last scan error doesn't come back when it is re-added.
func TestScansRemovedDropsError(t *testing.T) {
	l := newTestLib(t, Movies)
	if err := os.Remove(l.dir); err != nil {
		t.Fatal(err)
	}
	q := NewScans(l.scanner)
	runScans(t, q)
	q.Request(l.lib.ID)
	waitIdle(t, q)
	if q.Status()[l.lib.ID].Error == "" {
		t.Fatal("want a scan error")
	}
	q.Removed(l.lib.ID)
	if got := q.Status(); len(got) != 0 {
		t.Errorf("status after Removed = %+v, want empty", got)
	}
}

func TestScansRequestTwiceScansOnce(t *testing.T) {
	l := newTestLib(t, Movies)
	l.write("Heat (1995).mkv", "heat")
	q := NewScans(l.scanner)
	q.Request(l.lib.ID, l.lib.ID)
	q.Request(l.lib.ID)
	want := map[int64]ScanStatus{l.lib.ID: {State: ScanQueued}}
	if got := q.Status(); !reflect.DeepEqual(got, want) {
		t.Errorf("status = %+v, want %+v", got, want)
	}

	runScans(t, q)
	waitIdle(t, q)
	if n := l.prober.calls["Heat (1995).mkv"]; n != 1 {
		t.Errorf("probes = %d, want 1", n)
	}
	if _, ok := l.videos()["Heat (1995).mkv"]; !ok {
		t.Error("Heat not scanned")
	}
}

func TestScansRequestAllSkipsRemoved(t *testing.T) {
	l := newTestLib(t, Movies)
	var removed int64
	if err := l.db.QueryRow("INSERT INTO libraries (path, type, removed) VALUES ('Old', 'tv', 1) RETURNING id").
		Scan(&removed); err != nil {
		t.Fatal(err)
	}
	q := NewScans(l.scanner)
	if err := q.RequestAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := map[int64]ScanStatus{l.lib.ID: {State: ScanQueued}}
	if got := q.Status(); !reflect.DeepEqual(got, want) {
		t.Errorf("status = %+v, want only library %d queued", got, l.lib.ID)
	}
}

// A failed scan leaves its error in the status, and the queue goes on with the next library.
// A scan that works clears the error.
func TestScansError(t *testing.T) {
	l := newTestLib(t, Movies)
	var gone int64
	if err := l.db.QueryRow("INSERT INTO libraries (path, type) VALUES ('Gone', 'tv') RETURNING id").Scan(&gone); err != nil {
		t.Fatal(err)
	}
	l.write("Heat (1995).mkv", "heat")
	q := NewScans(l.scanner)
	runScans(t, q)
	q.Request(gone, l.lib.ID)
	waitIdle(t, q)

	st := q.Status()
	if st[gone].Error == "" || len(st) != 1 {
		t.Errorf("status = %+v, want an error for library %d only", st, gone)
	}
	if _, ok := l.videos()["Heat (1995).mkv"]; !ok {
		t.Error("Heat not scanned after the failed library")
	}

	if err := os.Mkdir(filepath.Join(l.scanner.Root, "Gone"), 0o755); err != nil {
		t.Fatal(err)
	}
	q.Request(gone)
	waitIdle(t, q)
	if st := q.Status(); len(st) != 0 {
		t.Errorf("status after a good scan = %+v, want empty", st)
	}
}
