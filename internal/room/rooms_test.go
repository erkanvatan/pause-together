package room

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/erkanvatan/pause-together/internal/library"
	"github.com/erkanvatan/pause-together/internal/media"
	"github.com/erkanvatan/pause-together/internal/store"
)

// fixture: Heat (1) has two audio tracks, a text and a picture subtitle, and a sidecar. Ronin (2) has
// one audio track and a sidecar. Gone (3) is missing. Old (4) is unplayable. Silent (5) has no audio.
const fixture = `
	INSERT INTO libraries (id, path, type) VALUES (1, 'Movies', 'movies');
	INSERT INTO videos (id, library_id, path, title, year, edition, version, season, episode, episode_end,
		episode_title, group_name, size, mtime, missing, video_codec, codec_string, unplayable)
	VALUES
		(1, 1, 'Heat (1995).mkv', 'Heat', 1995, '', '', 0, 0, 0, '', '', 10, 1, 0, 'h264', 'avc1.640028', NULL),
		(2, 1, 'Ronin (1998).mkv', 'Ronin', 1998, '', '', 0, 0, 0, '', '', 20, 1, 0, 'h264', 'avc1.640028', NULL),
		(3, 1, 'Gone (2000).mkv', 'Gone', 2000, '', '', 0, 0, 0, '', '', 30, 1, 1, 'h264', 'avc1.640028', NULL),
		(4, 1, 'Old (1990).avi', 'Old', 1990, '', '', 0, 0, 0, '', '', 40, 1, 0, 'mpeg4', '', 'codec'),
		(5, 1, 'Silent (1927).mkv', 'Silent', 1927, '', '', 0, 0, 0, '', '', 50, 1, 0, 'h264', 'avc1.640028', NULL);
	INSERT INTO audio_tracks (video_id, stream, codec, channels, layout, lang, title, is_default) VALUES
		(1, 1, 'aac', 2, 'stereo', 'eng', '', 1), (1, 2, 'eac3', 6, '5.1', 'tur', '', 0),
		(2, 1, 'aac', 2, 'stereo', 'eng', '', 1), (3, 1, 'aac', 2, 'stereo', 'eng', '', 1),
		(4, 1, 'mp3', 2, 'stereo', 'eng', '', 1);
	INSERT INTO subtitle_tracks (video_id, stream, codec, lang, title, is_default, forced, sdh, unavailable) VALUES
		(1, 3, 'subrip', 'eng', '', 0, 0, 0, NULL), (1, 4, 'hdmv_pgs_subtitle', 'tur', '', 0, 0, 0, 'image');
	INSERT INTO sidecar_subtitles (id, video_id, name, lang, forced, sdh, cache_key) VALUES
		(1, 1, 'Heat (1995).tr.srt', 'tr', 0, 0, '00000000000000000000000000000001'),
		(2, 2, 'Ronin (1998).tr.srt', 'tr', 0, 0, '00000000000000000000000000000002');`

func newTestRooms(t *testing.T) *Rooms {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), store.Migrations())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(fixture); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Movies"), 0o755); err != nil {
		t.Fatal(err)
	}
	// No worker runs, so jobs stay queued and Jobs.List shows every one.
	return &Rooms{DB: db, Library: &library.Libraries{DB: db, Root: root}, Jobs: media.NewJobs(t.TempDir(), nil)}
}

func stream(n int) *int { return &n }

func pick(video int64, audio *int) Pick { return Pick{VideoID: video, Audio: audio} }

// key is the job key of a video and audio track.
func key(t *testing.T, r *Rooms, video int64, audio *int) string {
	t.Helper()
	j, _, err := r.Library.PrepareJob(context.Background(), video, audio)
	if err != nil {
		t.Fatal(err)
	}
	return j.Key()
}

// jobs returns the queued keys, sorted.
func jobs(r *Rooms) []string {
	var keys []string
	for _, j := range r.Jobs.List() {
		keys = append(keys, j.Key)
	}
	slices.Sort(keys)
	return keys
}

func sorted(keys ...string) []string {
	slices.Sort(keys)
	return keys
}

// mustCreate creates a room and opens it, as its creator's page does.
func mustCreate(t *testing.T, r *Rooms, p Pick) Room {
	t.Helper()
	rm, err := r.Create(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Open(context.Background(), rm.ID); err != nil {
		t.Fatal(err)
	}
	return rm
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	r := newTestRooms(t)
	sidecar := int64(1)
	rm, err := r.Create(ctx, Pick{VideoID: 1, Audio: stream(2), Subtitle: &Subtitle{Sidecar: &sidecar}})
	if err != nil {
		t.Fatal(err)
	}
	if rm.ID == 0 || rm.Name != "" || rm.Video.ID != 1 || rm.Video.Title != "Heat" || *rm.Audio != 2 ||
		rm.Subtitle == nil || rm.Subtitle.Sidecar == nil || *rm.Subtitle.Sidecar != 1 || rm.Subtitle.Stream != nil ||
		rm.PositionMs != 0 || rm.Archived {
		t.Errorf("created %+v", rm)
	}
	// Opening it queues its prepare.
	if len(jobs(r)) != 0 {
		t.Errorf("created: jobs = %v, want none until opened", jobs(r))
	}
	if _, err := r.Open(ctx, rm.ID); err != nil {
		t.Fatal(err)
	}
	if got, want := jobs(r), []string{key(t, r, 1, stream(2))}; !slices.Equal(got, want) {
		t.Errorf("opened: jobs = %v, want %v", got, want)
	}

	silent := mustCreate(t, r, pick(5, nil))
	if silent.Audio != nil || silent.Subtitle != nil {
		t.Errorf("no audio, subtitle off: got %+v", silent)
	}

	list, err := r.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != silent.ID || list[1].ID != rm.ID {
		t.Errorf("list = %+v, want newest first", list)
	}
}

func TestBadPick(t *testing.T) {
	sidecarOfRonin, sidecarGone := int64(2), int64(9)
	tests := []struct {
		name string
		pick Pick
	}{
		{"unknown video", pick(99, stream(1))},
		{"missing video", pick(3, stream(1))},
		{"unplayable video", pick(4, stream(1))},
		{"audio not the video's", pick(1, stream(7))},
		{"no audio, but it has tracks", pick(1, nil)},
		{"audio for a video with none", pick(5, stream(1))},
		{"subtitle not the video's", Pick{VideoID: 1, Audio: stream(1), Subtitle: &Subtitle{Stream: stream(8)}}},
		{"picture subtitle", Pick{VideoID: 1, Audio: stream(1), Subtitle: &Subtitle{Stream: stream(4)}}},
		{"another video's sidecar", Pick{VideoID: 1, Audio: stream(1), Subtitle: &Subtitle{Sidecar: &sidecarOfRonin}}},
		{"unknown sidecar", Pick{VideoID: 1, Audio: stream(1), Subtitle: &Subtitle{Sidecar: &sidecarGone}}},
		{"empty subtitle", Pick{VideoID: 1, Audio: stream(1), Subtitle: &Subtitle{}}},
		{"stream and sidecar", Pick{VideoID: 1, Audio: stream(1),
			Subtitle: &Subtitle{Stream: stream(3), Sidecar: &sidecarOfRonin}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRooms(t)
			if _, err := r.Create(context.Background(), tt.pick); !errors.Is(err, ErrBadPick) {
				t.Errorf("create: err = %v, want ErrBadPick", err)
			}
			rm := mustCreate(t, r, pick(2, stream(1)))
			if _, err := r.Switch(context.Background(), rm.ID, tt.pick); !errors.Is(err, ErrBadPick) {
				t.Errorf("switch: err = %v, want ErrBadPick", err)
			}
		})
	}
}

func TestOpenQueuesOneJob(t *testing.T) {
	ctx := context.Background()
	r := newTestRooms(t)
	a := mustCreate(t, r, pick(1, stream(1)))
	b := mustCreate(t, r, pick(1, stream(1)))
	k := key(t, r, 1, stream(1))
	r.Jobs.Cancel(1, stream(1)) // as after a restart: nothing queued

	for _, id := range []int64{a.ID, b.ID, a.ID} {
		if _, err := r.Open(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if got := jobs(r); !slices.Equal(got, []string{k}) {
		t.Errorf("jobs = %v, want one: %v", got, k)
	}
	if _, err := r.Open(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown room: err = %v, want ErrNotFound", err)
	}
}

// A rescan can find the video gone, or a new file in its place no browser can play.
func TestUnusableVideoQueuesNothing(t *testing.T) {
	for _, update := range []string{
		"UPDATE videos SET missing = 1 WHERE id = 1",
		"UPDATE videos SET unplayable = 'codec' WHERE id = 1",
	} {
		t.Run(update, func(t *testing.T) {
			r := newTestRooms(t)
			rm := mustCreate(t, r, pick(1, stream(1)))
			r.Jobs.Cancel(1, stream(1))
			if _, err := r.DB.Exec(update); err != nil {
				t.Fatal(err)
			}
			got, err := r.Open(context.Background(), rm.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Video.Title != "Heat" {
				t.Errorf("video = %+v, want Heat", got.Video)
			}
			if len(jobs(r)) != 0 {
				t.Errorf("jobs = %v, want none", jobs(r))
			}
		})
	}
}

func TestArchivedRoom(t *testing.T) {
	ctx := context.Background()
	r := newTestRooms(t)
	rm := mustCreate(t, r, pick(1, stream(1)))
	if got, err := r.SetArchived(ctx, rm.ID, true); err != nil || !got.Archived {
		t.Fatalf("archive: %+v, %v", got, err)
	}
	if _, err := r.Open(ctx, rm.ID); err != nil {
		t.Fatal(err)
	}
	if len(jobs(r)) != 0 {
		t.Errorf("opened archived room: jobs = %v, want none", jobs(r))
	}
	if _, err := r.Switch(ctx, rm.ID, pick(2, stream(1))); !errors.Is(err, ErrArchived) {
		t.Errorf("switch: err = %v, want ErrArchived", err)
	}

	if got, err := r.SetArchived(ctx, rm.ID, false); err != nil || got.Archived {
		t.Fatalf("unarchive: %+v, %v", got, err)
	}
	if _, err := r.Open(ctx, rm.ID); err != nil {
		t.Fatal(err)
	}
	if got, want := jobs(r), []string{key(t, r, 1, stream(1))}; !slices.Equal(got, want) {
		t.Errorf("unarchived and opened: jobs = %v, want %v", got, want)
	}
	if _, err := r.SetArchived(ctx, 99, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown room: err = %v, want ErrNotFound", err)
	}
}

// A job is cancelled once no room needs it: switched away, archived or deleted. Another room on the
// same video and audio track keeps it, unless that room is archived.
func TestRelease(t *testing.T) {
	ctx := context.Background()
	actions := []struct {
		name string
		do   func(r *Rooms, id int64) error
		// after: the jobs left when no other room needs the old one.
		after func(t *testing.T, r *Rooms) []string
	}{
		{"switch to another video", func(r *Rooms, id int64) error {
			_, err := r.Switch(ctx, id, pick(2, stream(1)))
			return err
		}, func(t *testing.T, r *Rooms) []string { return []string{key(t, r, 2, stream(1))} }},
		{"switch to another audio track", func(r *Rooms, id int64) error {
			_, err := r.Switch(ctx, id, pick(1, stream(2)))
			return err
		}, func(t *testing.T, r *Rooms) []string { return []string{key(t, r, 1, stream(2))} }},
		{"archive", func(r *Rooms, id int64) error {
			_, err := r.SetArchived(ctx, id, true)
			return err
		}, func(*testing.T, *Rooms) []string { return nil }},
		{"delete", func(r *Rooms, id int64) error { return r.Delete(ctx, id) },
			func(*testing.T, *Rooms) []string { return nil }},
	}
	others := []struct {
		name     string
		other    bool // a second room on the same pick
		archived bool // that room is archived
		keepsOld bool
	}{
		{"alone", false, false, false},
		{"another room on it", true, false, true},
		{"an archived room on it", true, true, false},
	}
	for _, a := range actions {
		for _, o := range others {
			t.Run(a.name+", "+o.name, func(t *testing.T) {
				r := newTestRooms(t)
				rm := mustCreate(t, r, pick(1, stream(1)))
				if o.other {
					other := mustCreate(t, r, pick(1, stream(1)))
					if o.archived {
						if _, err := r.SetArchived(ctx, other.ID, true); err != nil {
							t.Fatal(err)
						}
					}
				}
				if err := a.do(r, rm.ID); err != nil {
					t.Fatal(err)
				}
				want := a.after(t, r)
				if o.keepsOld {
					want = append(want, key(t, r, 1, stream(1)))
				}
				if got := jobs(r); !slices.Equal(got, sorted(want...)) {
					t.Errorf("jobs = %v, want %v", got, sorted(want...))
				}
			})
		}
	}
}

func TestSwitchToSamePickKeepsJob(t *testing.T) {
	r := newTestRooms(t)
	rm := mustCreate(t, r, pick(1, stream(1)))
	if _, err := r.Switch(context.Background(), rm.ID, Pick{VideoID: 1, Audio: stream(1),
		Subtitle: &Subtitle{Stream: stream(3)}}); err != nil {
		t.Fatal(err)
	}
	if got, want := jobs(r), []string{key(t, r, 1, stream(1))}; !slices.Equal(got, want) {
		t.Errorf("jobs = %v, want %v", got, want)
	}
}

func TestSwitchResetsPosition(t *testing.T) {
	ctx := context.Background()
	r := newTestRooms(t)
	rm := mustCreate(t, r, Pick{VideoID: 1, Audio: stream(1), Subtitle: &Subtitle{Stream: stream(3)}})
	if _, err := r.DB.Exec("UPDATE rooms SET position_ms = 6000000 WHERE id = ?", rm.ID); err != nil {
		t.Fatal(err)
	}
	sidecar := int64(2)
	got, err := r.Switch(ctx, rm.ID, Pick{VideoID: 2, Audio: stream(1), Subtitle: &Subtitle{Sidecar: &sidecar}})
	if err != nil {
		t.Fatal(err)
	}
	if got.PositionMs != 0 || got.Video.ID != 2 || got.Subtitle == nil || got.Subtitle.Sidecar == nil ||
		*got.Subtitle.Sidecar != 2 || got.Subtitle.Stream != nil {
		t.Errorf("switched: %+v", got)
	}
	if _, err := r.Switch(ctx, 99, pick(2, stream(1))); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown room: err = %v, want ErrNotFound", err)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	r := newTestRooms(t)
	rm := mustCreate(t, r, pick(1, stream(1)))
	if err := r.Delete(ctx, rm.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Open(ctx, rm.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("open deleted room: err = %v, want ErrNotFound", err)
	}
	var videos int
	if err := r.DB.QueryRow("SELECT count(*) FROM videos").Scan(&videos); err != nil || videos != 5 {
		t.Errorf("videos = %d, %v; want all 5 kept", videos, err)
	}
	if err := r.Delete(ctx, rm.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete again: err = %v, want ErrNotFound", err)
	}
}

func TestRename(t *testing.T) {
	tests := []struct {
		name, want string
		err        error
	}{
		{"  Movie night  ", "Movie night", nil},
		{"Film gecesi ş", "Film gecesi ş", nil},
		{"", "", nil},
		{"   ", "", nil},
		{"😀", "", ErrBadName},
		{strings.Repeat("a", 33), "", ErrBadName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRooms(t)
			rm := mustCreate(t, r, pick(1, stream(1)))
			got, err := r.Rename(context.Background(), rm.ID, tt.name)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if err == nil && got.Name != tt.want {
				t.Errorf("name = %q, want %q", got.Name, tt.want)
			}
		})
	}
	r := newTestRooms(t)
	if _, err := r.Rename(context.Background(), 99, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown room: err = %v, want ErrNotFound", err)
	}
}

// The scan deletes a sidecar's row when its file goes. The room stays, with its subtitle off.
func TestSidecarGoneTurnsSubtitleOff(t *testing.T) {
	ctx := context.Background()
	r := newTestRooms(t)
	sidecar := int64(1)
	rm := mustCreate(t, r, Pick{VideoID: 1, Audio: stream(1), Subtitle: &Subtitle{Sidecar: &sidecar}})
	if _, err := r.DB.Exec("DELETE FROM sidecar_subtitles WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	got, err := r.Open(ctx, rm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subtitle != nil {
		t.Errorf("subtitle = %+v, want off", got.Subtitle)
	}
}

// A deleted room's id is never given to a new room: an old link or open tab must not lead to another
// room.
func TestDeletedIDNotReused(t *testing.T) {
	r := newTestRooms(t)
	mustCreate(t, r, pick(1, stream(1)))
	last := mustCreate(t, r, pick(1, stream(1)))
	if err := r.Delete(context.Background(), last.ID); err != nil {
		t.Fatal(err)
	}
	if next := mustCreate(t, r, pick(1, stream(1))); next.ID == last.ID {
		t.Errorf("new room got deleted room's id %d", last.ID)
	}
}
