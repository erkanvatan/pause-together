package library

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
)

// Scan states for ScanStatus.
const (
	ScanQueued   = "queued"
	ScanScanning = "scanning"
)

// ScanStatus is where a library stands in the scan queue.
type ScanStatus struct {
	State string `json:"state"` // ScanQueued, ScanScanning, or "" when neither
	Done  int    `json:"done"`  // while scanning: videos done so far
	Total int    `json:"total"` // while scanning: videos found
	Error string `json:"error"` // why the last scan failed; "" if it worked
}

// Scans runs library scans one at a time, in the order they were asked for.
type Scans struct {
	scanner *Scanner
	wake    chan struct{}

	mu      sync.Mutex
	queue   []int64
	current int64              // 0 when idle
	cancel  context.CancelFunc // stops the current scan
	done    int
	total   int
	errs    map[int64]string
}

// NewScans returns an empty queue that scans with s. Run works through it.
func NewScans(s *Scanner) *Scans {
	return &Scans{scanner: s, wake: make(chan struct{}, 1), errs: map[int64]string{}}
}

// Request queues scans of the given libraries. One already queued isn't queued twice; one being
// scanned right now is, since its files may have changed since the scan started.
func (q *Scans) Request(ids ...int64) {
	q.mu.Lock()
	for _, id := range ids {
		if !slices.Contains(q.queue, id) {
			q.queue = append(q.queue, id)
		}
	}
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default: // the worker is already woken
	}
}

// RequestAll queues a scan of every library that isn't removed.
func (q *Scans) RequestAll(ctx context.Context) error {
	libs, err := activeLibraries(ctx, q.scanner.DB)
	if err != nil {
		return err
	}
	ids := make([]int64, len(libs))
	for i, l := range libs {
		ids[i] = l.ID
	}
	q.Request(ids...)
	return nil
}

// Removed tells the queue a library was removed: its scan stops if it is running, and its last
// error is dropped. Otherwise a scan started before the removal could go on after a re-add, with
// the old type and an old list of the library's videos.
func (q *Scans) Removed(id int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.errs, id)
	if q.current == id {
		q.cancel()
	}
}

// Status returns the libraries that are queued, being scanned, or whose last scan failed.
func (q *Scans) Status() map[int64]ScanStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	st := make(map[int64]ScanStatus)
	for id, e := range q.errs {
		st[id] = ScanStatus{Error: e}
	}
	for _, id := range q.queue {
		st[id] = ScanStatus{State: ScanQueued, Error: q.errs[id]}
	}
	if q.current != 0 {
		st[q.current] = ScanStatus{State: ScanScanning, Done: q.done, Total: q.total, Error: q.errs[q.current]}
	}
	return st
}

// Run scans queued libraries until ctx is cancelled.
func (q *Scans) Run(ctx context.Context) {
	for {
		id, scanCtx, ok := q.next(ctx)
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-q.wake:
				continue
			}
		}
		q.scan(scanCtx, id)
		if ctx.Err() != nil {
			return
		}
	}
}

// next takes the first queued library and makes it the current one. Its scan runs with the returned
// context, which Removed cancels.
func (q *Scans) next(ctx context.Context) (int64, context.Context, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return 0, nil, false
	}
	id := q.queue[0]
	q.queue = q.queue[1:]
	scanCtx, cancel := context.WithCancel(ctx)
	q.current, q.cancel, q.done, q.total = id, cancel, 0, 0
	return id, scanCtx, true
}

func (q *Scans) scan(ctx context.Context, id int64) {
	lib, err := getLibrary(ctx, q.scanner.DB, id)
	if err == nil {
		err = q.scanner.ScanLibrary(ctx, lib, q.progress)
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	switch {
	case err == nil, errors.Is(err, ErrNotFound), errors.Is(err, ErrRemoved):
		delete(q.errs, id) // done, or removed while queued or scanning
	case ctx.Err() != nil:
		// Shutdown, or the library was removed; the next start or a re-add scans it again.
	default:
		slog.Error("scan library", "library", lib.Path, "err", err)
		q.errs[id] = err.Error()
	}
	q.cancel()
	q.current, q.cancel = 0, nil
}

func (q *Scans) progress(done, total int) {
	q.mu.Lock()
	q.done, q.total = done, total
	q.mu.Unlock()
}
