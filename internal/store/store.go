// Package store holds the SQLite database: opening, migrations and backups.
package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"modernc.org/sqlite"

	"github.com/erkanvatan/pause-together/internal/user"
)

const (
	// busyTimeout is how long a connection waits for another one's write lock before failing.
	busyTimeout = 5 * time.Second

	// A backup is made when the newest one is at least backupEvery old, checked every backupCheck.
	backupEvery = 24 * time.Hour
	backupCheck = time.Hour
	// BackupKeep is how many backups are kept; older ones are deleted.
	BackupKeep = 7
)

// name_key(name) is user.NameKey in SQL, for the migration that merges users by name. SQLite's lower()
// and NOCASE fold only ASCII. The driver adds it to each connection opened after this.
func init() {
	if err := sqlite.RegisterDeterministicScalarFunction("name_key", 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			name, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("name_key: want text, got %T", args[0])
			}
			return user.NameKey(name), nil
		},
	); err != nil {
		panic(err) // only fails for a bad name or argument count
	}
}

// all: so .gitkeep is embedded too; an empty folder won't embed.
//
//go:embed all:migrations
var migrationFiles embed.FS

// Migrations returns the app's migration files.
func Migrations() fs.FS {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		panic(err) // "migrations" is a valid path; this can't happen
	}
	return sub
}

// Open opens the database at path, making its folder if needed, and applies migrations.
func Open(ctx context.Context, path string, migrations fs.FS) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, db, migrations); err != nil {
		return nil, errors.Join(fmt.Errorf("migrate: %w", err), db.Close())
	}
	return db, nil
}

// dsn sets up every connection the pool opens:
//   - foreign_keys: off by default in SQLite, and cascades silently don't run without it.
//   - WAL: readers don't block the writer.
//   - busy_timeout: wait for a lock instead of failing at once.
//   - _txlock=immediate: take the write lock at BEGIN. A transaction that reads, then writes, would
//     otherwise fail with SQLITE_BUSY at once, without waiting.
func dsn(path string) string {
	q := url.Values{
		// busy_timeout first: the driver runs these in order, and journal_mode may need to wait for a lock.
		"_pragma": {
			"busy_timeout(" + strconv.FormatInt(busyTimeout.Milliseconds(), 10) + ")",
			"foreign_keys(1)",
			"journal_mode(WAL)",
		},
		"_txlock": {"immediate"},
	}
	return path + "?" + q.Encode()
}

// InTx runs fn in a transaction. It commits when fn returns nil and rolls back otherwise.
func InTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			// ErrTxDone: a cancelled ctx has rolled it back already.
			if rbErr := tx.Rollback(); !errors.Is(rbErr, sql.ErrTxDone) {
				err = errors.Join(err, rbErr)
			}
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
