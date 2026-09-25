package store

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// openTest opens a fresh database in a temp dir with the given migrations.
func openTest(t *testing.T, migrations fs.FS) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "test.db")
	db, err := Open(context.Background(), path, migrations)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

func TestPoolPragmas(t *testing.T) {
	ctx := context.Background()
	db, _ := openTest(t, fstest.MapFS{
		"0001_a.sql": {Data: []byte("CREATE TABLE a (id INTEGER PRIMARY KEY);")},
	})

	// The migration's connection went back to the pool; it must be the only one so far,
	// so the first connection taken below is that one.
	if n := db.Stats().OpenConnections; n != 1 {
		t.Fatalf("open connections after Open = %d, want 1", n)
	}

	// Hold several at once, so each is a different connection.
	for i := range 3 {
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()

		checks := []struct {
			pragma string
			want   string
		}{
			{"foreign_keys", "1"},
			{"journal_mode", "wal"},
			{"busy_timeout", "5000"},
		}
		for _, c := range checks {
			var got string
			if err := conn.QueryRowContext(ctx, "PRAGMA "+c.pragma).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("conn %d: %s = %q, want %q", i, c.pragma, got, c.want)
			}
		}
	}
}

func TestInTx(t *testing.T) {
	ctx := context.Background()
	db, _ := openTest(t, fstest.MapFS{
		"0001_a.sql": {Data: []byte("CREATE TABLE a (id INTEGER PRIMARY KEY);")},
	})
	insert := func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO a DEFAULT VALUES")
		return err
	}

	if err := InTx(ctx, db, insert); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	err := InTx(ctx, db, func(tx *sql.Tx) error {
		if err := insert(tx); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}

	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM a").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("rows = %d, want 1: the first committed, the second rolled back", n)
	}
}
