package store

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// upTo returns the real migrations numbered n and below.
func upTo(t *testing.T, n int) fstest.MapFS {
	t.Helper()
	all, err := loadMigrations(Migrations())
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{}
	for _, m := range all[:n] {
		body, err := fs.ReadFile(Migrations(), m.name)
		if err != nil {
			t.Fatal(err)
		}
		files[m.name] = &fstest.MapFile{Data: body}
	}
	return files
}

// Tracks probed before migration 4 get their unavailable reason from their codec.
func TestMigration4MarksOldSubtitleTracks(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(ctx, path, upTo(t, 3))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"INSERT INTO libraries (id, path, type) VALUES (1, 'Movies', 'movies')",
		`INSERT INTO videos (id, library_id, path, title, year, edition, version, season, episode, episode_end,
			episode_title, group_name, size, mtime) VALUES (1, 1, 'A (2020).mkv', 'A', 2020, '', '', 0, 0, 0, '', '', 1, 1)`,
		`INSERT INTO subtitle_tracks (video_id, stream, codec, lang, title, is_default, forced, sdh) VALUES
			(1, 2, 'subrip', '', '', 0, 0, 0), (1, 3, 'hdmv_pgs_subtitle', '', '', 0, 0, 0), (1, 4, 'eia_608', '', '', 0, 0, 0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(ctx, path, Migrations())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	want := map[int]string{2: "", 3: "image", 4: "codec"}
	for stream, w := range want {
		var got string
		if err := db.QueryRow("SELECT COALESCE(unavailable, '') FROM subtitle_tracks WHERE stream = ?", stream).
			Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != w {
			t.Errorf("stream %d: unavailable = %q, want %q", stream, got, w)
		}
	}
}

// Migration 10 drops the "was here" list. Rooms and their chat stay.
func TestMigration10DropsRoomVisitors(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(ctx, path, upTo(t, 9))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"INSERT INTO users (id, token_hash, name) VALUES (1, x'01', 'Alice')",
		"INSERT INTO libraries (id, path, type) VALUES (1, 'Movies', 'movies')",
		`INSERT INTO videos (id, library_id, path, title, year, edition, version, season, episode, episode_end,
			episode_title, group_name, size, mtime) VALUES (1, 1, 'A (2020).mkv', 'A', 2020, '', '', 0, 0, 0, '', '', 1, 1)`,
		"INSERT INTO rooms (id, name, video_id) VALUES (1, 'Movie night', 1)",
		"INSERT INTO messages (room_id, user_id, text, video_id, position_ms, sent_at) VALUES (1, 1, 'hi', 1, 0, 0)",
		"INSERT INTO room_visitors (room_id, user_id) VALUES (1, 1)",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(ctx, path, Migrations())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	counts := []struct {
		query string
		want  int
	}{
		{"SELECT count(*) FROM sqlite_master WHERE name = 'room_visitors'", 0},
		{"SELECT count(*) FROM rooms WHERE name = 'Movie night'", 1},
		{"SELECT count(*) FROM messages WHERE text = 'hi'", 1},
	}
	for _, c := range counts {
		var n int
		if err := db.QueryRow(c.query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != c.want {
			t.Errorf("%s = %d, want %d", c.query, n, c.want)
		}
	}
}
