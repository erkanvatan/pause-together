package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func sqlFile(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	err := db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = ?", name).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestMigrateInOrderOnce(t *testing.T) {
	ctx := context.Background()
	migrations := fstest.MapFS{
		// Out of order in the map on purpose; the second needs the first's table.
		"0002_insert.sql": sqlFile("INSERT INTO a (id) VALUES (1);"),
		"0001_create.sql": sqlFile("CREATE TABLE a (id INTEGER PRIMARY KEY);"),
		"README.md":       sqlFile("not a migration"),
	}
	path := filepath.Join(t.TempDir(), "test.db")

	for range 2 {
		db, err := Open(ctx, path, migrations)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if v := userVersion(t, db); v != 2 {
			t.Errorf("user_version = %d, want 2", v)
		}
		if n := count(t, db, "a"); n != 1 {
			t.Errorf("rows = %d, want 1 (a migration ran twice?)", n)
		}
		_ = db.Close()
	}
}

func TestMigrateFailureRollsBack(t *testing.T) {
	tests := []struct {
		name string
		bad  string // the second migration
	}{
		{
			name: "bad statement after a good one",
			bad:  "CREATE TABLE b (id INTEGER PRIMARY KEY); INSERT INTO nope VALUES (1);",
		},
		{
			name: "foreign key check fails",
			bad: `CREATE TABLE b (id INTEGER PRIMARY KEY, a_id INTEGER NOT NULL REFERENCES a(id));
			      INSERT INTO b (id, a_id) VALUES (1, 99);`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "test.db")
			good := fstest.MapFS{"0001_a.sql": sqlFile("CREATE TABLE a (id INTEGER PRIMARY KEY);")}
			db, err := Open(ctx, path, good)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			_ = db.Close()

			withBad := fstest.MapFS{
				"0001_a.sql": good["0001_a.sql"],
				"0002_b.sql": sqlFile(tt.bad),
			}
			if db, err := Open(ctx, path, withBad); err == nil {
				_ = db.Close()
				t.Fatal("Open succeeded, want error")
			}

			db, err = Open(ctx, path, good)
			if err != nil {
				t.Fatalf("reopen: %v", err)
			}
			defer func() { _ = db.Close() }()
			if v := userVersion(t, db); v != 1 {
				t.Errorf("user_version = %d, want 1", v)
			}
			if tableExists(t, db, "b") {
				t.Error("table b exists; the failed migration was not rolled back")
			}
		})
	}
}

func TestMigrateRebuildKeepsChildren(t *testing.T) {
	db, _ := openTest(t, fstest.MapFS{
		"0001_tables.sql": sqlFile(`
			CREATE TABLE parent (id INTEGER PRIMARY KEY);
			CREATE TABLE child (
				id INTEGER PRIMARY KEY,
				parent_id INTEGER NOT NULL REFERENCES parent(id) ON DELETE CASCADE
			);
			INSERT INTO parent (id) VALUES (1), (2);
			INSERT INTO child (id, parent_id) VALUES (10, 1), (11, 1), (12, 2);`),
		// SQLite's 12-step table rebuild: new table, copy, drop old, rename.
		"0002_rebuild.sql": sqlFile(`
			CREATE TABLE parent_new (id INTEGER PRIMARY KEY, name TEXT NOT NULL DEFAULT '');
			INSERT INTO parent_new (id) SELECT id FROM parent;
			DROP TABLE parent;
			ALTER TABLE parent_new RENAME TO parent;`),
	})

	if n := count(t, db, "child"); n != 3 {
		t.Errorf("child rows = %d, want 3; the rebuild cascaded", n)
	}
	// The foreign key still works after the rebuild.
	if _, err := db.Exec("DELETE FROM parent WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, "child"); n != 1 {
		t.Errorf("child rows after deleting parent 1 = %d, want 1", n)
	}
}

func TestMigrationFilesInvalid(t *testing.T) {
	create := "CREATE TABLE a (id INTEGER PRIMARY KEY);"
	tests := []struct {
		name  string
		files fstest.MapFS
	}{
		{"gap", fstest.MapFS{"0001_a.sql": sqlFile(create), "0003_c.sql": sqlFile(create)}},
		{"duplicate", fstest.MapFS{"0001_a.sql": sqlFile(create), "0001_b.sql": sqlFile(create)}},
		{"starts at 2", fstest.MapFS{"0002_a.sql": sqlFile(create)}},
		{"no number", fstest.MapFS{"init.sql": sqlFile(create)}},
		{"no underscore", fstest.MapFS{"0001.sql": sqlFile(create)}},
		{"zero", fstest.MapFS{"0000_a.sql": sqlFile(create)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := loadMigrations(tt.files); err == nil {
				t.Error("loadMigrations succeeded, want error")
			}
		})
	}
}

func TestMigrateDatabaseAhead(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(ctx, path, fstest.MapFS{
		"0001_a.sql": sqlFile("CREATE TABLE a (id INTEGER PRIMARY KEY);"),
		"0002_b.sql": sqlFile("CREATE TABLE b (id INTEGER PRIMARY KEY);"),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = db.Close()

	// An older binary that only knows the first migration.
	older := fstest.MapFS{"0001_a.sql": sqlFile("CREATE TABLE a (id INTEGER PRIMARY KEY);")}
	if db, err := Open(ctx, path, older); err == nil {
		_ = db.Close()
		t.Fatal("Open succeeded on a newer database, want error")
	}
}

func TestEmbeddedMigrations(t *testing.T) {
	if _, err := loadMigrations(Migrations()); err != nil {
		t.Fatalf("embedded migrations: %v", err)
	}
}

func TestApplySkipsWhenAlreadyApplied(t *testing.T) {
	ctx := context.Background()
	// As if another process applied migration 1 between our read of user_version and our BEGIN.
	db, _ := openTest(t, fstest.MapFS{"0001_a.sql": sqlFile("CREATE TABLE a (id INTEGER PRIMARY KEY);")})
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	if err := apply(ctx, conn, 1, "CREATE TABLE a (id INTEGER PRIMARY KEY);"); err != nil {
		t.Errorf("apply of an applied migration: %v, want skipped", err)
	}
	if v := userVersion(t, db); v != 1 {
		t.Errorf("user_version = %d, want 1", v)
	}
}
