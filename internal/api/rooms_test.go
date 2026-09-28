package api

import (
	"net/http"
	"strings"
	"testing"
)

// seedVideo adds video 7, Heat (1995), with one stereo audio track (stream 1), to a library.
func seedVideo(t *testing.T, d Deps, libraryID int64) {
	t.Helper()
	if _, err := d.Libraries.DB.Exec(`
		INSERT INTO videos (id, library_id, path, title, year, edition, version, season, episode, episode_end,
			episode_title, group_name, size, mtime, codec_string)
		VALUES (7, ?, 'Heat (1995).mkv', 'Heat', 1995, '', '', 0, 0, 0, '', '', 1, 1, 'avc1.640028');
		INSERT INTO audio_tracks (video_id, stream, codec, channels, layout, lang, title, is_default)
		VALUES (7, 1, 'aac', 2, 'stereo', 'eng', '', 1);`, libraryID); err != nil {
		t.Fatal(err)
	}
}

// The room routes work on the guest port; deleting a room is for the admin port only. Each step runs
// on the state the steps before it left.
func TestRooms(t *testing.T) {
	d := adminDeps(t)
	if _, err := d.Libraries.DB.Exec("INSERT INTO libraries (id, path, type) VALUES (1, 'Movies', 'movies')"); err != nil {
		t.Fatal(err)
	}
	seedVideo(t, d, 1)
	guest, admin := Guest(fakeBuild(), d), Admin(fakeBuild(), d)

	const heat = `"video":{"id":7,"type":"movies","title":"Heat","year":1995,"edition":"","version":"","season":0,` +
		`"episode":0,"episodeEnd":0,"episodeTitle":"","group":"","durationMs":0,"codecString":"avc1.640028",` +
		`"unplayable":"","appleOnly":false,"missing":false}`
	steps := []struct {
		name         string
		h            http.Handler
		method, path string
		body         string
		wantStatus   int
		want         string // the whole body; "" = not checked
	}{
		{"create", guest, http.MethodPost, "/api/rooms", `{"videoId":7,"audio":1,"subtitle":null}`, http.StatusCreated,
			`{"id":1,"name":"",` + heat + `,"audio":1,"subtitle":null,"positionMs":0,"archived":false}`},
		{"create, bad pick", guest, http.MethodPost, "/api/rooms", `{"videoId":7,"audio":2}`, http.StatusBadRequest,
			`{"error":"bad-pick"}`},
		{"create, bad JSON", guest, http.MethodPost, "/api/rooms", `{`, http.StatusBadRequest, `{"error":"bad-request"}`},
		{"list", guest, http.MethodGet, "/api/rooms", "", http.StatusOK,
			`[{"id":1,"name":"",` + heat + `,"audio":1,"subtitle":null,"positionMs":0,"archived":false}]`},
		{"open", guest, http.MethodGet, "/api/rooms/1", "", http.StatusOK, ""},
		{"open unknown", guest, http.MethodGet, "/api/rooms/2", "", http.StatusNotFound, `{"error":"not-found"}`},
		{"open malformed", guest, http.MethodGet, "/api/rooms/x", "", http.StatusNotFound, `{"error":"not-found"}`},
		{"rename", guest, http.MethodPut, "/api/rooms/1/name", `{"name":" Movie night "}`, http.StatusOK, ""},
		{"rename, bad name", guest, http.MethodPut, "/api/rooms/1/name", `{"name":"😀"}`, http.StatusBadRequest,
			`{"error":"bad-name"}`},
		{"rename unknown", guest, http.MethodPut, "/api/rooms/2/name", `{"name":"x"}`, http.StatusNotFound, `{"error":"not-found"}`},
		{"switch", guest, http.MethodPut, "/api/rooms/1/video", `{"videoId":7,"audio":1}`, http.StatusOK, ""},
		{"switch, bad pick", guest, http.MethodPut, "/api/rooms/1/video", `{"videoId":8,"audio":1}`,
			http.StatusBadRequest, `{"error":"bad-pick"}`},
		{"switch unknown", guest, http.MethodPut, "/api/rooms/2/video", `{"videoId":7,"audio":1}`, http.StatusNotFound, `{"error":"not-found"}`},
		{"archive", guest, http.MethodPost, "/api/rooms/1/archive", "", http.StatusOK,
			`{"id":1,"name":"Movie night",` + heat + `,"audio":1,"subtitle":null,"positionMs":0,"archived":true}`},
		{"switch archived", guest, http.MethodPut, "/api/rooms/1/video", `{"videoId":7,"audio":1}`,
			http.StatusConflict, `{"error":"archived"}`},
		{"unarchive", guest, http.MethodPost, "/api/rooms/1/unarchive", "", http.StatusOK, ""},
		{"archive unknown", guest, http.MethodPost, "/api/rooms/2/archive", "", http.StatusNotFound, `{"error":"not-found"}`},
		{"delete on guest port", guest, http.MethodDelete, "/api/admin/rooms/1", "", http.StatusNotFound, ""},
		{"delete", admin, http.MethodDelete, "/api/admin/rooms/1", "", http.StatusNoContent, ""},
		{"delete again", admin, http.MethodDelete, "/api/admin/rooms/1", "", http.StatusNotFound, `{"error":"not-found"}`},
		{"list after delete", guest, http.MethodGet, "/api/rooms", "", http.StatusOK, `[]`},
	}
	for _, s := range steps {
		rec := adminRequest(s.h, s.method, s.path, s.body)
		if rec.Code != s.wantStatus {
			t.Errorf("%s: status = %d (body %q), want %d", s.name, rec.Code, rec.Body.String(), s.wantStatus)
		}
		if got := strings.TrimSpace(rec.Body.String()); s.want != "" && got != s.want {
			t.Errorf("%s: body = %s\nwant %s", s.name, got, s.want)
		}
	}
}
