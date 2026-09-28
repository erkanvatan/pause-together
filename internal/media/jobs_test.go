package media

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePreparer writes a partial file, reports half the duration done, then waits for the test: a
// value on finish ends the job with that error, a cancelled ctx ends it with ctx's error.
type fakePreparer struct {
	started chan Job
	finish  chan error
	// afterCancel, if not nil, holds a cancelled job until it's closed, like an ffmpeg slow to stop.
	afterCancel chan struct{}
}

func (f *fakePreparer) Prepare(ctx context.Context, j Job, dir string, progress func(time.Duration)) error {
	out := filepath.Join(dir, videoFile)
	if err := os.WriteFile(out, []byte("partial"), 0o644); err != nil {
		return err
	}
	progress(j.Duration / 2)
	f.started <- j
	select {
	case <-ctx.Done():
		if f.afterCancel != nil {
			<-f.afterCancel
		}
		return ctx.Err()
	case err := <-f.finish:
		if err == nil {
			err = os.WriteFile(out, []byte("video"), 0o644)
		}
		return err
	}
}

type testJobs struct {
	*Jobs
	prep *fakePreparer
	dir  string
}

// newTestJobs returns a queue with plenty of free space. start runs its worker until the test ends.
func newTestJobs(t *testing.T) *testJobs {
	t.Helper()
	dir := t.TempDir()
	prep := &fakePreparer{started: make(chan Job), finish: make(chan error)}
	j := NewJobs(dir, prep)
	j.FreeSpace = func(string) (uint64, error) { return 1 << 40, nil }
	return &testJobs{Jobs: j, prep: prep, dir: dir}
}

func (q *testJobs) start(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() { q.Run(ctx) })
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
}

func (q *testJobs) started(t *testing.T) Job {
	t.Helper()
	select {
	case j := <-q.prep.started:
		return j
	case <-time.After(5 * time.Second):
		t.Fatal("job never started")
		return Job{}
	}
}

// tmpLeft lists what's left in the cache dir with a .tmp name.
func (q *testJobs) tmpLeft(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		t.Fatal(err)
	}
	var tmp []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			tmp = append(tmp, e.Name())
		}
	}
	return tmp
}

// waitFor polls cond until it holds, failing the test after 5 s.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// cancel cancels a job by its video and audio stream, as rooms do.
func (q *testJobs) cancel(j Job) {
	q.Cancel(j.VideoID, &j.Audio.Stream)
}

// Cancel goes by video and audio stream: a job queued before its file changed goes too, and another
// audio stream of the same video stays.
func TestJobsCancelByVideo(t *testing.T) {
	q := newTestJobs(t)
	q.start(t)
	q.Add(testJob(1))
	q.started(t)
	queued := testJob(2)
	other := testJob(2)
	other.Audio = &AudioTrack{Stream: 2}
	noAudio := testJob(2)
	noAudio.Audio = nil
	q.Add(queued)
	q.Add(other)
	q.Add(noAudio)

	changed := queued
	changed.Size, changed.Mtime = 2000, 2 // the file as it is now: another key
	q.Cancel(changed.VideoID, &changed.Audio.Stream)
	var keys []string
	for _, s := range q.List() {
		keys = append(keys, s.Key)
	}
	if want := []string{testJob(1).Key(), other.Key(), noAudio.Key()}; !slices.Equal(keys, want) {
		t.Errorf("after cancel: %v, want %v", keys, want)
	}
	q.Cancel(2, nil)
	if l := q.List(); len(l) != 2 {
		t.Errorf("after cancelling no audio: %+v, want jobs 1 and 2's other stream", l)
	}
	q.cancel(testJob(1))
}

func testJob(id int64) Job {
	return Job{VideoID: id, Name: "video " + string(rune('0'+id)), Source: "/media/x.mkv",
		Duration: 10 * time.Second, Size: 1000, Mtime: 1, Audio: &AudioTrack{Stream: 1}}
}

func TestJobsPrepare(t *testing.T) {
	q := newTestJobs(t)
	q.start(t)
	job := testJob(1)
	q.Add(job)
	q.started(t)
	q.prep.finish <- nil
	waitFor(t, "the queue to empty", func() bool { return len(q.List()) == 0 })

	f, err := q.Open(job.Key(), videoFile)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 16)
	n, _ := f.Read(buf)
	if got := string(buf[:n]); got != "video" {
		t.Errorf("prepared file = %q, want %q", got, "video")
	}
	if tmp := q.tmpLeft(t); len(tmp) > 0 {
		t.Errorf("left behind %v", tmp)
	}
}

func TestJobsCancelMidJob(t *testing.T) {
	q := newTestJobs(t)
	q.start(t)
	job := testJob(1)
	q.Add(job)
	q.started(t)
	if tmp := q.tmpLeft(t); len(tmp) != 1 {
		t.Fatalf("tmp while running = %v, want one", tmp)
	}
	q.cancel(job)
	waitFor(t, "the queue to empty", func() bool { return len(q.List()) == 0 })
	waitFor(t, "the tmp folder to go", func() bool { return len(q.tmpLeft(t)) == 0 })
	if _, err := q.Open(job.Key(), videoFile); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open after cancel: err = %v, want not exist", err)
	}
}

// A room switches away and straight back while the cancelled run is still stopping.
func TestJobsAddAgainWhileCancelling(t *testing.T) {
	q := newTestJobs(t)
	q.prep.afterCancel = make(chan struct{})
	q.start(t)
	job := testJob(1)
	q.Add(job)
	q.started(t)
	q.cancel(job)
	q.Add(job)
	close(q.prep.afterCancel)
	q.started(t) // runs again
	q.prep.finish <- nil
	waitFor(t, "the copy", func() bool { return q.ready(job.Key()) })
}

func TestJobsKeyFolderWithoutVideo(t *testing.T) {
	q := newTestJobs(t)
	job := testJob(1)
	if err := os.MkdirAll(filepath.Join(q.dir, job.Key(), "leftover"), 0o755); err != nil {
		t.Fatal(err)
	}
	q.start(t)
	q.Add(job)
	q.started(t)
	q.prep.finish <- nil
	waitFor(t, "the queue to empty", func() bool { return len(q.List()) == 0 })
	if !q.ready(job.Key()) {
		t.Errorf("no copy, list = %+v", q.List())
	}
}

func TestJobsCancelQueued(t *testing.T) {
	q := newTestJobs(t)
	q.start(t)
	q.Add(testJob(1))
	q.started(t)
	q.Add(testJob(2))
	q.cancel(testJob(2))
	if l := q.List(); len(l) != 1 || l[0].Key != testJob(1).Key() {
		t.Errorf("list = %+v, want only job 1", l)
	}
}

func TestJobsNotEnoughSpace(t *testing.T) {
	q := newTestJobs(t)
	job := testJob(1)
	job.Size = 5 << 30
	q.FreeSpace = func(string) (uint64, error) { return 5<<30 + freeSpaceMargin - 1, nil }
	q.start(t)
	q.Add(job)
	waitFor(t, "the job to fail", func() bool {
		l := q.List()
		return len(l) == 1 && l[0].State == JobFailed
	})
	st := q.List()[0]
	if st.Error != FailNoSpace || !strings.Contains(st.Detail, "GB") {
		t.Errorf("status = %+v, want error %q with sizes in the detail", st, FailNoSpace)
	}
	select {
	case <-q.prep.started:
		t.Error("ffmpeg ran without enough space")
	default:
	}
	if tmp := q.tmpLeft(t); len(tmp) > 0 {
		t.Errorf("left behind %v", tmp)
	}
}

func TestJobsFailedThenRetried(t *testing.T) {
	q := newTestJobs(t)
	q.start(t)
	job := testJob(1)
	q.Add(job)
	q.started(t)
	q.prep.finish <- errors.New("ffmpeg: exit status 1: boom")
	waitFor(t, "the job to fail", func() bool {
		l := q.List()
		return len(l) == 1 && l[0].State == JobFailed
	})
	if st := q.List()[0]; st.Error != FailPrepare || !strings.Contains(st.Detail, "boom") || st.Name != job.Name {
		t.Errorf("status = %+v, want error %q with ffmpeg's message", st, FailPrepare)
	}
	if tmp := q.tmpLeft(t); len(tmp) > 0 {
		t.Errorf("left behind %v", tmp)
	}

	q.Add(job) // asked for again: tried again
	q.started(t)
	if l := q.List(); len(l) != 1 || l[0].State != JobRunning {
		t.Errorf("list = %+v, want the job running again", l)
	}
	q.prep.finish <- nil
}

func TestJobsFailedRetriedWithoutSubtitles(t *testing.T) {
	q := newTestJobs(t)
	q.start(t)
	job := testJob(1)
	job.Subtitles = []int{3, 4}
	q.Add(job)
	if j := q.started(t); len(j.Subtitles) != 2 {
		t.Errorf("first run subtitles = %v, want both", j.Subtitles)
	}
	q.prep.finish <- errors.New("ffmpeg: exit status 1: bad subtitle")
	if j := q.started(t); j.Subtitles != nil {
		t.Errorf("second run subtitles = %v, want none", j.Subtitles)
	}
	q.prep.finish <- nil
	waitFor(t, "the queue to empty", func() bool { return len(q.List()) == 0 })
	if !q.ready(job.Key()) {
		t.Errorf("no copy after the run without subtitles")
	}
}

func TestJobsCancelDropsFailure(t *testing.T) {
	q := newTestJobs(t)
	q.start(t)
	job := testJob(1)
	q.Add(job)
	q.started(t)
	q.prep.finish <- errors.New("boom")
	waitFor(t, "the job to fail", func() bool { return len(q.List()) == 1 && q.List()[0].State == JobFailed })
	q.cancel(job)
	if l := q.List(); len(l) != 0 {
		t.Errorf("list = %+v, want empty", l)
	}
}

func TestJobsPlaceInLine(t *testing.T) {
	q := newTestJobs(t)
	q.start(t)
	q.Add(testJob(1))
	q.started(t)
	q.Add(testJob(2))
	q.Add(testJob(3))
	q.Add(testJob(2)) // already queued
	q.Add(testJob(1)) // already running

	want := []JobStatus{
		{Key: testJob(1).Key(), Name: testJob(1).Name, State: JobRunning, Progress: 0.5},
		{Key: testJob(2).Key(), Name: testJob(2).Name, State: JobQueued, Place: 1},
		{Key: testJob(3).Key(), Name: testJob(3).Name, State: JobQueued, Place: 2},
	}
	got := q.List()
	if len(got) != len(want) {
		t.Fatalf("list = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("list[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	q.cancel(testJob(2))
	if l := q.List(); len(l) != 2 || l[1].Key != testJob(3).Key() || l[1].Place != 1 {
		t.Errorf("after cancel: list = %+v, want job 3 first in line", l)
	}
	q.cancel(testJob(1))
	q.started(t) // job 3
}

func TestJobsStatus(t *testing.T) {
	q := newTestJobs(t)
	q.start(t)
	running, queued, none := testJob(1), testJob(2), testJob(3)
	q.Add(running)
	q.started(t)
	q.Add(queued)
	for _, tt := range []struct {
		job  Job
		want JobStatus
	}{
		{running, JobStatus{Key: running.Key(), Name: running.Name, State: JobRunning, Progress: 0.5}},
		{queued, JobStatus{Key: queued.Key(), Name: queued.Name, State: JobQueued, Place: 1}},
		{none, JobStatus{Key: none.Key()}},
	} {
		if got := q.Status(tt.job.Key()); got != tt.want {
			t.Errorf("Status(%s) = %+v, want %+v", tt.job.Name, got, tt.want)
		}
	}
	q.prep.finish <- nil
	waitFor(t, "job 1 to be ready", func() bool { return q.Status(running.Key()).State == JobReady })
	q.started(t) // job 2
	q.cancel(queued)
}

func TestJobsAddDone(t *testing.T) {
	q := newTestJobs(t)
	job := testJob(1)
	if err := os.MkdirAll(filepath.Join(q.dir, job.Key()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(q.dir, job.Key(), videoFile), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	q.Add(job)
	if l := q.List(); len(l) != 0 {
		t.Errorf("list = %+v, want nothing queued for a prepared video", l)
	}
}

func TestJobsRemoveStaleTmpAtStart(t *testing.T) {
	q := newTestJobs(t)
	for _, p := range []string{"aaaa.tmp/video.mp4", "bbbb/video.mp4", "loose.tmp"} {
		p = filepath.Join(q.dir, p)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	q.start(t)
	waitFor(t, "stale tmp to go", func() bool { return len(q.tmpLeft(t)) == 0 })
	if _, err := os.Stat(filepath.Join(q.dir, "bbbb", "video.mp4")); err != nil {
		t.Errorf("a finished copy was touched: %v", err)
	}
}

func TestJobsOpenBadKey(t *testing.T) {
	q := newTestJobs(t)
	job := testJob(1)
	if err := os.MkdirAll(filepath.Join(q.dir, job.Key()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(q.dir, job.Key(), videoFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	upper := strings.ToUpper(job.Key())
	for _, key := range []string{"", "..", "../x", job.Key()[:31], job.Key() + "0", upper, testJob(2).Key(),
		job.Key() + ".tmp"} {
		if f, err := q.Open(key, videoFile); !errors.Is(err, fs.ErrNotExist) {
			if f != nil {
				_ = f.Close()
			}
			t.Errorf("Open(%q): err = %v, want not exist", key, err)
		}
	}
	for _, name := range []string{"", ".", "..", "../x", "video.mp4.part", "subtitle.vtt.part", "x.vtt", "2.vtt/x",
		"/video.mp4", "-1.vtt"} {
		if f, err := q.Open(job.Key(), name); !errors.Is(err, fs.ErrNotExist) {
			if f != nil {
				_ = f.Close()
			}
			t.Errorf("Open(key, %q): err = %v, want not exist", name, err)
		}
	}
	f, err := q.Open(job.Key(), videoFile)
	if err != nil {
		t.Fatalf("Open(key, %q): %v", videoFile, err)
	}
	_ = f.Close()
}

func TestJobsDisk(t *testing.T) {
	q := newTestJobs(t)
	for p, size := range map[string]int{"a/video.mp4": 1000, "b.tmp/video.mp4": 234} {
		p = filepath.Join(q.dir, p)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cache, free, err := q.Disk()
	if err != nil {
		t.Fatal(err)
	}
	if cache != 1234 || free != 1<<40 {
		t.Errorf("Disk() = %d, %d; want 1234, %d", cache, free, uint64(1<<40))
	}
}

func TestFreeSpace(t *testing.T) {
	free, err := freeSpace(t.TempDir())
	if err != nil || free == 0 {
		t.Errorf("freeSpace = %d, %v; want > 0", free, err)
	}
}
