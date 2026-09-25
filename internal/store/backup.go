package store

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	backupPrefix = "pausetogether-"
	backupTime   = "20060102-150405" // UTC; sorts by name in time order
	backupSuffix = ".db"
)

// Backups copies the database into Dir with VACUUM INTO. Never a plain file copy: with WAL, a copy of
// the live file can be broken.
type Backups struct {
	DB   *sql.DB
	Dir  string
	Keep int
	Now  func() time.Time
}

// Run makes a backup now if due, then checks again every hour until ctx is done. Errors are logged.
func (b *Backups) Run(ctx context.Context) {
	t := time.NewTicker(backupCheck)
	defer t.Stop()
	for {
		if _, err := b.maybe(ctx); err != nil && ctx.Err() == nil {
			slog.Error("backup", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// maybe makes a backup unless the newest is under backupEvery old, then deletes all but the newest Keep.
func (b *Backups) maybe(ctx context.Context) (made bool, err error) {
	if err := os.MkdirAll(b.Dir, 0o755); err != nil {
		return false, err
	}
	names, err := b.list()
	if err != nil {
		return false, err
	}
	now := b.Now().UTC()
	// Newest first. A backup named in the future (made while the clock was wrong) is skipped, or it
	// would count as fresh until that date and stop all backups.
	for _, name := range slices.Backward(names) {
		t, _ := backupTimeOf(name)
		if t.After(now) {
			continue
		}
		if now.Sub(t) < backupEvery {
			return false, nil
		}
		break
	}

	name := backupPrefix + now.Format(backupTime) + backupSuffix
	final := filepath.Join(b.Dir, name)
	// Write to .tmp and rename, so a half-written file never counts as the newest backup.
	// Clear any left by a crash or shutdown mid-backup; each is a full database copy.
	tmp := final + ".tmp"
	leftovers, err := filepath.Glob(filepath.Join(b.Dir, backupPrefix+"*"+backupSuffix+".tmp"))
	if err != nil {
		return false, err
	}
	for _, f := range leftovers {
		if err := os.Remove(f); err != nil {
			return false, err
		}
	}
	if _, err := b.DB.ExecContext(ctx, "VACUUM INTO ?", tmp); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, final); err != nil {
		return false, err
	}
	slog.Info("backup made", "file", final)

	names = append(names, name)
	for _, old := range names[:max(0, len(names)-b.Keep)] {
		if err := os.Remove(filepath.Join(b.Dir, old)); err != nil {
			return true, err
		}
	}
	return true, nil
}

// list returns the backup file names in Dir, oldest first. Other files are left out.
func (b *Backups) list() ([]string, error) {
	entries, err := os.ReadDir(b.Dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if _, ok := backupTimeOf(e.Name()); ok && e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

// backupTimeOf reads the time from a backup file name.
func backupTimeOf(name string) (time.Time, bool) {
	s, ok := strings.CutPrefix(name, backupPrefix)
	if !ok {
		return time.Time{}, false
	}
	s, ok = strings.CutSuffix(s, backupSuffix)
	if !ok {
		return time.Time{}, false
	}
	t, err := time.Parse(backupTime, s)
	return t, err == nil
}
