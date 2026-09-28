package api

import (
	"net/http"
	"strings"
	"testing"
)

// The picker's routes work on the guest port.
func TestVideosOnGuestPort(t *testing.T) {
	d := adminDeps(t)
	if _, err := d.Libraries.DB.Exec(`
		INSERT INTO libraries (id, path, type) VALUES (1, 'Movies', 'movies');
		INSERT INTO videos (id, library_id, path, title, year, edition, version, season, episode, episode_end,
			episode_title, group_name, size, mtime, codec_string)
		VALUES (7, 1, 'Heat (1995).mkv', 'Heat', 1995, '', '', 0, 0, 0, '', '', 1, 1, 'avc1.640028');
		INSERT INTO audio_tracks (video_id, stream, codec, channels, layout, lang, title, is_default)
		VALUES (7, 1, 'aac', 2, 'stereo', 'eng', '', 1);`); err != nil {
		t.Fatal(err)
	}
	h := Guest(fakeBuild(), d)

	tests := []struct {
		target     string
		wantStatus int
		want       string
	}{
		{"/api/videos", http.StatusOK, `[{"id":7,"type":"movies","title":"Heat","year":1995,"edition":"",` +
			`"version":"","season":0,"episode":0,"episodeEnd":0,"episodeTitle":"","group":"","durationMs":0,` +
			`"codecString":"avc1.640028","unplayable":"","appleOnly":false}]`},
		{"/api/videos/7", http.StatusOK, `{"id":7,"type":"movies","title":"Heat","year":1995,"edition":"",` +
			`"version":"","season":0,"episode":0,"episodeEnd":0,"episodeTitle":"","group":"","durationMs":0,` +
			`"codecString":"avc1.640028","unplayable":"","appleOnly":false,"missing":false,` +
			`"audio":[{"stream":1,"codec":"aac","channels":2,"lang":"en","title":"","default":true}],` +
			`"subtitles":[],"sidecars":[]}`},
		{"/api/videos/8", http.StatusNotFound, ""},
		{"/api/videos/x", http.StatusNotFound, ""},
		{"/api/languages", http.StatusOK, `{"audio":"","subtitles":[]}`},
	}
	for _, tt := range tests {
		rec := adminRequest(h, http.MethodGet, tt.target, "")
		if rec.Code != tt.wantStatus {
			t.Errorf("%s: status = %d, want %d", tt.target, rec.Code, tt.wantStatus)
		}
		if got := strings.TrimSpace(rec.Body.String()); tt.want != "" && got != tt.want {
			t.Errorf("%s: body = %s\nwant %s", tt.target, got, tt.want)
		}
	}
}

func TestPutLanguages(t *testing.T) {
	h := Admin(fakeBuild(), adminDeps(t))
	tests := []struct {
		name, body string
		wantStatus int
		want       string
	}{
		{"saved normalized", `{"audio":"tur","subtitles":["TR","eng"]}`, http.StatusOK,
			`{"audio":"tr","subtitles":["tr","en"]}`},
		{"bad code", `{"audio":"","subtitles":["english"]}`, http.StatusBadRequest, `{"error":"bad-lang"}`},
		{"bad JSON", `{`, http.StatusBadRequest, `{"error":"bad-request"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := adminRequest(h, http.MethodPut, "/api/admin/languages", tt.body)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
	// The failed saves left the first one in place.
	rec := adminRequest(h, http.MethodGet, "/api/languages", "")
	if got, want := strings.TrimSpace(rec.Body.String()), `{"audio":"tr","subtitles":["tr","en"]}`; got != want {
		t.Errorf("after: %s, want %s", got, want)
	}
}
