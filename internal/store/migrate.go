package store

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
)

type migration struct {
	version int
	name    string
}

// loadMigrations lists the NNNN_name.sql files in fsys, sorted. Versions must run 1, 2, 3… with no
// gaps or duplicates. Other files are ignored.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, err
	}
	var ms []migration
	for _, name := range names {
		num, _, ok := strings.Cut(name, "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil || v < 1 {
			return nil, fmt.Errorf("migration %q: name must be NNNN_name.sql", name)
		}
		ms = append(ms, migration{version: v, name: name})
	}
	slices.SortFunc(ms, func(a, b migration) int { return cmp.Compare(a.version, b.version) })
	for i, m := range ms {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration %q: expected version %d (gap or duplicate)", m.name, i+1)
		}
	}
	return ms, nil
}

// migrate applies the migrations newer than the database's user_version, each in its own transaction.
//
// It runs on one dedicated connection with foreign_keys off. Rebuilding a table (make new, copy, drop
// old) would otherwise run ON DELETE CASCADE on the drop. foreign_key_check catches what that lets
// through, before each commit.
func migrate(ctx context.Context, db *sql.DB, fsys fs.FS) (err error) {
	ms, err := loadMigrations(fsys)
	if err != nil {
		return err
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, conn.Close()) }()

	// Outside a transaction: inside one, setting foreign_keys does nothing.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	// Back on before the connection returns to the pool. If this fails, Open fails and closes the pool.
	defer func() {
		if _, onErr := conn.ExecContext(context.WithoutCancel(ctx), "PRAGMA foreign_keys = ON"); onErr != nil {
			err = errors.Join(err, fmt.Errorf("turn foreign_keys back on: %w", onErr))
		}
	}()

	var current int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current > len(ms) {
		return fmt.Errorf("database is at version %d, newer than this binary's %d", current, len(ms))
	}

	for _, m := range ms[current:] {
		body, err := fs.ReadFile(fsys, m.name)
		if err != nil {
			return err
		}
		if err := apply(ctx, conn, m.version, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
	}
	return nil
}

// apply runs one migration and sets user_version in the same transaction.
func apply(ctx context.Context, conn *sql.Conn, version int, body string) (err error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, tx.Rollback())
		}
	}()

	// Read again under the write lock: another process on the same file may have applied it already.
	var current int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current >= version {
		return tx.Rollback()
	}

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	// PRAGMA takes no bound parameters; version is an int, so this is safe.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	if err := foreignKeyCheck(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// foreignKeyCheck fails if any row points at a parent that doesn't exist.
func foreignKeyCheck(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var table, parent string
		var rowid sql.NullInt64
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return err
		}
		return fmt.Errorf("foreign key check: a row in %s points at a missing row in %s", table, parent)
	}
	return rows.Err()
}
