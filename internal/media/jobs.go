package media

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Job states for JobStatus.
const (
	JobRunning = "running"
	JobQueued  = "queued"
	JobFailed  = "failed"
)

// Why a job failed, as codes: the web strings file turns them into text.
const (
	FailNoSpace = "no-space" // not enough free disk space to start
	FailPrepare = "failed"   // ffmpeg, or writing the cache, failed
)

// freeSpaceMargin is how much disk a job leaves free on top of the copy's size (about the source's
// size), so the database and backups can still write.
const freeSpaceMargin = 1 << 30

// videoFile is the prepared video's name inside its key's folder.
const videoFile = "video.mp4"

// tmpSuffix marks a folder a job is still writing. It's renamed to the bare key when done, so a
// half-written copy never looks finished.
const tmpSuffix = ".tmp"

var (
	keyPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	// cacheFilePattern matches the files a key's folder may hold: videoFile, sidecarFile and
	// subtitleName's.
	cacheFilePattern = regexp.MustCompile(`^(?:` + regexp.QuoteMeta(videoFile) + `|` +
		regexp.QuoteMeta(sidecarFile) + `|[0-9]+` + regexp.QuoteMeta(subtitleExt) + `)$`)
)

func validKey(key string) bool { return keyPattern.MatchString(key) }

var errNoSpace = errors.New("not enough free disk space")

// JobStatus is one job on the admin page.
type JobStatus struct {
	Key      string  `json:"key"`
	Name     string  `json:"name"`
	State    string  `json:"state"`    // JobRunning, JobQueued or JobFailed
	Place    int     `json:"place"`    // queued: 1 = next in line
	Progress float64 `json:"progress"` // running: 0 to 1
	Error    string  `json:"error"`    // failed: FailNoSpace or FailPrepare
	Detail   string  `json:"detail"`   // failed: sizes, or ffmpeg's message
}

// Jobs prepares videos into the cache folder, one at a time, in the order they were asked for. Each
// copy lives in its own folder, named by its job's key.
type Jobs struct {
	dir      string
	preparer Preparer
	// FreeSpace reports the free bytes on dir's disk. Tests swap it.
	FreeSpace func(dir string) (uint64, error)
	wake      chan struct{}

	mu      sync.Mutex
	queue   []Job
	current *Job
	cancel  context.CancelFunc // stops the current job
	// cancelled: Cancel stopped the current job, which may still be winding down. Adding its key again
	// queues it anew instead of counting the dying run.
	cancelled bool
	done      time.Duration // how much of the current job's video is done
	failed    []failedJob
}

// failedJob keeps its job, so Cancel can find it by video.
type failedJob struct {
	job    Job
	status JobStatus
}

// NewJobs returns an empty queue that prepares into dir with p. Run works through it.
func NewJobs(dir string, p Preparer) *Jobs {
	return &Jobs{dir: dir, preparer: p, FreeSpace: freeSpace, wake: make(chan struct{}, 1)}
}

// Add queues a job, unless its copy is ready or it is queued or running already. A job that failed
// before is tried again.
func (q *Jobs) Add(j Job) {
	key := j.Key()
	if q.ready(key) {
		return
	}
	q.mu.Lock()
	q.failed = slices.DeleteFunc(q.failed, func(f failedJob) bool { return f.status.Key == key })
	running := q.current != nil && !q.cancelled && q.current.Key() == key
	if running || slices.ContainsFunc(q.queue, func(queued Job) bool { return queued.Key() == key }) {
		q.mu.Unlock()
		return
	}
	q.queue = append(q.queue, j)
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default: // the worker is already woken
	}
}

// Cancel drops the jobs of a video and audio stream (nil: no audio) nobody needs any more: out of the
// queue, stopped if running, and their failures forgotten. It goes by video, not key: the file may have
// changed since a job was queued, and with it the key. A copy that is already ready stays.
func (q *Jobs) Cancel(videoID int64, audio *int) {
	match := func(j Job) bool {
		if j.VideoID != videoID || (j.Audio == nil) != (audio == nil) {
			return false
		}
		return audio == nil || j.Audio.Stream == *audio
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = slices.DeleteFunc(q.queue, match)
	q.failed = slices.DeleteFunc(q.failed, func(f failedJob) bool { return match(f.job) })
	if q.current != nil && match(*q.current) {
		q.cancel()
		q.cancelled = true
	}
}

// List returns the running job, then the queued ones in order, then the failed ones.
func (q *Jobs) List() []JobStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	list := []JobStatus{}
	if q.current != nil {
		var progress float64
		if q.current.Duration > 0 {
			progress = min(1, float64(q.done)/float64(q.current.Duration))
		}
		list = append(list, JobStatus{Key: q.current.Key(), Name: q.current.Name, State: JobRunning, Progress: progress})
	}
	for i, j := range q.queue {
		list = append(list, JobStatus{Key: j.Key(), Name: j.Name, State: JobQueued, Place: i + 1})
	}
	for _, f := range q.failed {
		list = append(list, f.status)
	}
	return list
}

// Open opens a file of key's copy in the cache: a prepared video, one of its subtitle tracks, or a
// converted sidecar. A malformed key or name, or one with no ready file, gives an error that matches
// fs.ErrNotExist.
func (q *Jobs) Open(key, name string) (*os.File, error) {
	if !validKey(key) || !cacheFilePattern.MatchString(name) {
		return nil, fs.ErrNotExist
	}
	return os.Open(filepath.Join(q.dir, key, name))
}

// Disk returns the bytes in the cache folder, half-written copies included, and the free bytes on
// its disk.
func (q *Jobs) Disk() (cache int64, free uint64, err error) {
	err = filepath.WalkDir(q.dir, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // a job finished or was cancelled mid-walk
		}
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		cache += info.Size()
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	free, err = q.FreeSpace(q.dir)
	return cache, free, err
}

// Run prepares queued jobs until ctx is cancelled. It first deletes what jobs left half-written when
// the app last stopped.
func (q *Jobs) Run(ctx context.Context) {
	q.removeTmp()
	for {
		j, jobCtx, ok := q.next(ctx)
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-q.wake:
				continue
			}
		}
		err := q.prepare(jobCtx, j)
		q.finish(jobCtx, j, err)
		if ctx.Err() != nil {
			return
		}
	}
}

func (q *Jobs) removeTmp() {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		slog.Error("clean cache", "err", err)
		return
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), tmpSuffix) {
			if err := os.RemoveAll(filepath.Join(q.dir, e.Name())); err != nil {
				slog.Error("clean cache", "err", err)
			}
		}
	}
}

// next takes the first queued job and makes it the current one. It runs with the returned context,
// which Cancel cancels.
func (q *Jobs) next(ctx context.Context) (Job, context.Context, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return Job{}, nil, false
	}
	j := q.queue[0]
	q.queue = q.queue[1:]
	jobCtx, cancel := context.WithCancel(ctx)
	q.current, q.cancel, q.done, q.cancelled = &j, cancel, 0, false
	return j, jobCtx, true
}

// prepare writes the job's copy into a .tmp folder and renames it when done. When the run fails, it
// runs once more without subtitles: one broken subtitle track mustn't cost a video that plays fine.
// On any error, the .tmp folder is deleted.
func (q *Jobs) prepare(ctx context.Context, j Job) error {
	key := j.Key()
	if q.ready(key) {
		return nil // finished after Add checked for it
	}
	free, err := q.FreeSpace(q.dir)
	if err != nil {
		return err
	}
	if need := uint64(max(j.Size, 0)) + freeSpaceMargin; free < need {
		return fmt.Errorf("%w: needs %s, %s free", errNoSpace, gigabytes(need), gigabytes(free))
	}

	// Leftovers of a run that couldn't clean up, or a key folder without its video, would make Mkdir or
	// Rename fail on every try.
	tmp, final := filepath.Join(q.dir, key+tmpSuffix), filepath.Join(q.dir, key)
	err = os.RemoveAll(final)
	if err == nil {
		err = q.run(ctx, j, tmp)
	}
	if err != nil && ctx.Err() == nil && len(j.Subtitles) > 0 {
		slog.Warn("prepare failed, trying again without subtitles", "video", j.Name, "err", err)
		j.Subtitles = nil
		err = q.run(ctx, j, tmp)
	}
	if err == nil {
		err = os.Rename(tmp, final)
	}
	if err != nil {
		if rmErr := os.RemoveAll(tmp); rmErr != nil {
			slog.Error("delete half-written copy", "err", rmErr)
		}
	}
	return err
}

// run prepares j into a fresh tmp folder.
func (q *Jobs) run(ctx context.Context, j Job, tmp string) error {
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.Mkdir(tmp, 0o755); err != nil {
		return err
	}
	return q.preparer.Prepare(ctx, j, tmp, q.progress)
}

func (q *Jobs) finish(ctx context.Context, j Job, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	switch {
	case err == nil:
	case ctx.Err() != nil:
		// Cancelled, or shutdown; rooms ask for it again when they need it.
	case errors.Is(err, errNoSpace):
		slog.Error("prepare", "video", j.Name, "err", err)
		q.failed = append(q.failed, failedJob{j, JobStatus{Key: j.Key(), Name: j.Name, State: JobFailed,
			Error: FailNoSpace, Detail: strings.TrimPrefix(err.Error(), errNoSpace.Error()+": ")}})
	default:
		slog.Error("prepare", "video", j.Name, "err", err)
		q.failed = append(q.failed, failedJob{j, JobStatus{Key: j.Key(), Name: j.Name, State: JobFailed,
			Error: FailPrepare, Detail: err.Error()}})
	}
	q.cancel()
	q.current, q.cancel = nil, nil
}

func (q *Jobs) progress(done time.Duration) {
	q.mu.Lock()
	q.done = done
	q.mu.Unlock()
}

// ready reports whether key's copy is in the cache.
func (q *Jobs) ready(key string) bool {
	_, err := os.Stat(filepath.Join(q.dir, key, videoFile))
	return err == nil
}

// freeSpace returns the bytes an unprivileged user can still write on dir's disk.
func freeSpace(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}

func gigabytes(b uint64) string {
	return fmt.Sprintf("%.1f GB", float64(b)/1e9)
}
