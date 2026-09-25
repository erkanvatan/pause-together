package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
	"time"
)

// fakeClock is a settable time source.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newBackups(t *testing.T) (*Backups, *fakeClock) {
	t.Helper()
	db, _ := openTest(t, fstest.MapFS{
		"0001_a.sql": sqlFile("CREATE TABLE a (id INTEGER PRIMARY KEY); INSERT INTO a (id) VALUES (42);"),
	})
	clock := &fakeClock{t: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	return &Backups{DB: db, Dir: filepath.Join(t.TempDir(), "backups"), Keep: 7, Now: clock.now}, clock
}

func backupNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestBackupSchedule(t *testing.T) {
	ctx := context.Background()
	b, clock := newBackups(t)

	steps := []struct {
		name     string
		advance  time.Duration
		wantMade bool
	}{
		{"on start", 0, true},
		{"1 h later: fresh, skip", time.Hour, false},
		{"23 h 59 m after the first: skip", 22*time.Hour + 59*time.Minute, false},
		{"24 h after the first: make", time.Minute, true},
		{"right after: skip", 0, false},
	}
	for _, s := range steps {
		clock.t = clock.t.Add(s.advance)
		made, err := b.maybe(ctx)
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if made != s.wantMade {
			t.Errorf("%s: made = %v, want %v", s.name, made, s.wantMade)
		}
	}

	want := []string{"pausetogether-20260924-120000.db", "pausetogether-20260925-120000.db"}
	if got := backupNames(t, b.Dir); !slices.Equal(got, want) {
		t.Errorf("backups = %v, want %v", got, want)
	}
}

func TestBackupKeepsNewest(t *testing.T) {
	ctx := context.Background()
	b, clock := newBackups(t)
	if err := os.MkdirAll(b.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Not a backup: must never be deleted.
	other := filepath.Join(b.Dir, "notes.txt")
	if err := os.WriteFile(other, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	for day := range 9 {
		clock.t = time.Date(2026, 9, 1+day, 3, 0, 0, 0, time.UTC)
		if made, err := b.maybe(ctx); err != nil || !made {
			t.Fatalf("day %d: made = %v, err = %v", day, made, err)
		}
	}

	want := []string{"notes.txt"} // sorts first by name
	for day := 3; day <= 9; day++ {
		want = append(want, time.Date(2026, 9, day, 3, 0, 0, 0, time.UTC).Format("pausetogether-20060102-150405.db"))
	}
	if got := backupNames(t, b.Dir); !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestBackupIsReadable(t *testing.T) {
	b, _ := newBackups(t)
	if _, err := b.maybe(context.Background()); err != nil {
		t.Fatal(err)
	}
	names := backupNames(t, b.Dir)
	if len(names) != 1 {
		t.Fatalf("backups = %v, want one", names)
	}

	db, err := sql.Open("sqlite", filepath.Join(b.Dir, names[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var id int
	if err := db.QueryRow("SELECT id FROM a").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != 42 {
		t.Errorf("id = %d, want 42", id)
	}
}

func TestBackupClearsLeftoverTmp(t *testing.T) {
	b, _ := newBackups(t)
	if err := os.MkdirAll(b.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Left by a shutdown mid-backup, under an older time than the next backup's.
	leftover := filepath.Join(b.Dir, "pausetogether-20260901-000000.db.tmp")
	if err := os.WriteFile(leftover, []byte("half"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.maybe(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"pausetogether-20260924-120000.db"}
	if got := backupNames(t, b.Dir); !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestBackupFromTheFutureIgnored(t *testing.T) {
	b, _ := newBackups(t)
	if err := os.MkdirAll(b.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Made while the clock was wrong.
	future := filepath.Join(b.Dir, "pausetogether-20300101-000000.db")
	if err := os.WriteFile(future, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	made, err := b.maybe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !made {
		t.Error("made = false; a backup named in the future blocked a real one")
	}
}
