package media

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Job states for JobStatus.
const (
	JobReady   = "ready" // Status only: the copy is in the cache
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

// The host's clean-up setting, in days, stays within these: a prepared copy nobody has used for that
// long is deleted. Opening a room prepares it again. The database checks the same bounds.
const (
	MinUnusedDays = 1
	MaxUnusedDays = 365
)

// ErrBadDays means a clean-up setting outside MinUnusedDays to MaxUnusedDays.
var ErrBadDays = errors.New("unused days out of range")

// CleanInterval is how often Clean runs.
const CleanInterval = time.Hour

// oldSuffix marks a copy Clean is deleting. It ends in tmpSuffix, so the start-up clean-up deletes what
// a stop left behind, and it can't clash with a job's own tmp folder.
const oldSuffix = ".old" + tmpSuffix

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
// copy lives in its own folder, named by its job's key. The folder's mtime is when the copy was last
// used, not its video's: http.ServeContent sends the video's mtime as Last-Modified.
type Jobs struct {
	dir      string
	db       *sql.DB // holds the clean-up setting
	preparer Preparer
	// FreeSpace reports the free bytes on dir's disk. Tests swap it.
	FreeSpace func(dir string) (uint64, error)
	// Now is the clock for when copies were used. Tests swap it.
	Now  func() time.Time
	wake chan struct{}

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

// NewJobs returns an empty queue that prepares into dir with p. Run works through it. db holds the
// clean-up setting.
func NewJobs(dir string, db *sql.DB, p Preparer) *Jobs {
	return &Jobs{dir: dir, db: db, preparer: p, FreeSpace: freeSpace, Now: time.Now, wake: make(chan struct{}, 1)}
}

// Add queues a job, unless its copy is ready (then the copy counts as used) or it is queued or running
// already. A job that failed before is tried again.
func (q *Jobs) Add(j Job) {
	key := j.Key()
	q.mu.Lock()
	// Under the lock, so Clean can't delete the copy between the check and the touch.
	if q.ready(key) {
		q.touch(key)
		q.mu.Unlock()
		return
	}
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

// Status returns where key's copy stands: JobReady once it is in the cache, else its job as List shows
// it, else a JobStatus with no State: no job.
func (q *Jobs) Status(key string) JobStatus {
	if q.ready(key) {
		return JobStatus{Key: key, State: JobReady}
	}
	for _, j := range q.List() {
		if j.Key == key {
			return j
		}
	}
	return JobStatus{Key: key}
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

// Subtitles returns the embedded subtitle streams key's copy has as WebVTT, in order. A copy made by the
// run without subtitles has none, whatever its video's track list says. Never nil.
func (q *Jobs) Subtitles(key string) []int {
	streams := []int{}
	if !validKey(key) {
		return streams
	}
	entries, err := os.ReadDir(filepath.Join(q.dir, key))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Error("list prepared subtitles", "err", err)
	}
	for _, e := range entries {
		n, ok := strings.CutSuffix(e.Name(), subtitleExt)
		if s, err := strconv.Atoi(n); ok && err == nil && cacheFilePattern.MatchString(e.Name()) {
			streams = append(streams, s)
		}
	}
	slices.Sort(streams)
	return streams
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

// Touch marks key's copy as used now, so Clean keeps it. A missing copy is left missing.
func (q *Jobs) Touch(key string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.touch(key)
}

// touch is Touch, with q.mu held.
func (q *Jobs) touch(key string) {
	if !validKey(key) {
		return
	}
	now := q.Now()
	if err := os.Chtimes(filepath.Join(q.dir, key), now, now); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Error("mark copy used", "err", err)
	}
}

// UnusedDays returns the clean-up setting: a copy nobody has used for this many days is deleted.
func (q *Jobs) UnusedDays(ctx context.Context) (int, error) {
	var days int
	err := q.db.QueryRowContext(ctx, "SELECT unused_days FROM cache_settings").Scan(&days)
	return days, err
}

// SetUnusedDays saves the clean-up setting. The next Clean uses it. Days outside MinUnusedDays to
// MaxUnusedDays fail with ErrBadDays.
func (q *Jobs) SetUnusedDays(ctx context.Context, days int) error {
	if days < MinUnusedDays || days > MaxUnusedDays {
		return ErrBadDays
	}
	_, err := q.db.ExecContext(ctx, "UPDATE cache_settings SET unused_days = ?", days)
	return err
}

// Clean deletes the prepared copies nobody has used for UnusedDays. A converted sidecar shares the
// cache folder but belongs to the scan, and a .tmp folder to a running job: both stay.
func (q *Jobs) Clean(ctx context.Context) {
	days, err := q.UnusedDays(ctx)
	if err != nil {
		slog.Error("clean cache", "err", err)
		return
	}
	cutoff := q.Now().Add(-time.Duration(days) * 24 * time.Hour)
	q.remove(q.take(func(lastUsed time.Time) bool { return lastUsed.Before(cutoff) }))
}

// Clear deletes every prepared copy, used or not. Like Clean, it leaves converted sidecars and a
// running job's .tmp folder alone. Anyone streaming a deleted copy is cut off; opening the room again
// prepares it anew.
func (q *Jobs) Clear() {
	q.remove(q.take(func(time.Time) bool { return true }))
}

// remove deletes the copies take moved out of the way.
func (q *Jobs) remove(old []string) {
	for _, p := range old {
		if err := os.RemoveAll(p); err != nil {
			slog.Error("delete copy", "err", err)
		}
	}
}

// take renames the copies whose last use drop approves out of the way, and returns their new paths.
// Under q.mu, so Add can't touch a copy in between; deleting a copy of many GB waits until the lock is
// free.
func (q *Jobs) take(drop func(lastUsed time.Time) bool) []string {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		slog.Error("clean cache", "err", err)
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	var old []string
	for _, e := range entries {
		key := e.Name()
		if !validKey(key) || !q.ready(key) {
			continue
		}
		// Read again under the lock: an Add may have touched it since ReadDir.
		info, err := e.Info()
		if err != nil || !drop(info.ModTime()) {
			continue
		}
		p := filepath.Join(q.dir, key+oldSuffix)
		if err := os.Rename(filepath.Join(q.dir, key), p); err != nil {
			slog.Error("delete copy", "err", err)
			continue
		}
		slog.Info("deleting copy", "key", key, "last used", info.ModTime())
		old = append(old, p)
	}
	return old
}

// CleanEvery runs Clean every interval until ctx is done. Not at start: a clock that is wrong at boot,
// before NTP fixes it, would make every copy look unused.
func (q *Jobs) CleanEvery(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			q.Clean(ctx)
		}
	}
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
	if err == nil {
		// The folder's mtime is the run's start, hours ago for a long video.
		q.Touch(key)
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
