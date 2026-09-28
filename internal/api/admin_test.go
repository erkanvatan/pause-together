package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erkanvatan/pause-together/internal/library"
	"github.com/erkanvatan/pause-together/internal/media"
	"github.com/erkanvatan/pause-together/internal/room"
)

// adminDeps adds Libraries, Scans, Jobs and Rooms over a media folder holding two empty folders, Movies and TV.
// No scan worker runs, so requested scans stay queued. The cache folder is empty.
func adminDeps(t *testing.T) Deps {
	t.Helper()
	users := testUsers(t)
	root := t.TempDir()
	for _, d := range []string{"Movies", "TV"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	d := deps(users)
	d.Libraries = &library.Libraries{DB: users.DB, Root: root}
	d.Scans = library.NewScans(&library.Scanner{DB: users.DB, Root: root})
	d.Jobs = media.NewJobs(t.TempDir(), media.FFmpeg{})
	d.Rooms = &room.Rooms{DB: users.DB, Library: d.Libraries, Jobs: d.Jobs}
	return d
}

// adminRequest sends a same-origin request with a Host the admin listener accepts.
func adminRequest(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = "localhost:8081"
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	return serve(h, req)
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestAdminRoutesOnlyOnAdminPort(t *testing.T) {
	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/api/admin/libraries", ""},
		{http.MethodPost, "/api/admin/libraries", `{"path":"Movies","type":"movies"}`},
		{http.MethodDelete, "/api/admin/libraries/1", ""},
		{http.MethodPost, "/api/admin/libraries/1/scan", ""},
		{http.MethodGet, "/api/admin/folders", ""},
		{http.MethodGet, "/api/admin/problems", ""},
		{http.MethodGet, "/api/admin/jobs", ""},
		{http.MethodPut, "/api/admin/languages", `{"audio":"","subtitles":["tr"]}`},
		{http.MethodDelete, "/api/admin/rooms/1", ""},
	}
	build := fakeBuild()
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			d := adminDeps(t)
			if rec := adminRequest(Guest(build, d), rt.method, rt.path, rt.body); rec.Code != http.StatusNotFound {
				t.Errorf("guest: status = %d, want 404", rec.Code)
			}
			admin := Admin(build, d)
			// So the delete and scan routes find library 1.
			if rec := adminRequest(admin, http.MethodPost, "/api/admin/libraries", `{"path":"TV","type":"tv"}`); rec.Code != http.StatusCreated {
				t.Fatalf("add: status = %d", rec.Code)
			}
			// So the room delete finds room 1.
			seedVideo(t, d, 1)
			if rec := adminRequest(admin, http.MethodPost, "/api/rooms", `{"videoId":7,"audio":1}`); rec.Code != http.StatusCreated {
				t.Fatalf("create room: status = %d (body %q)", rec.Code, rec.Body.String())
			}
			if rec := adminRequest(admin, rt.method, rt.path, rt.body); rec.Code >= 400 {
				t.Errorf("admin: status = %d (body %q), want success", rec.Code, rec.Body.String())
			}
		})
	}
}

type libraryBody struct {
	ID     int64  `json:"id"`
	Path   string `json:"path"`
	Type   string `json:"type"`
	Videos int    `json:"videos"`
	Scan   struct {
		State string `json:"state"`
	} `json:"scan"`
}

func TestAdminLibraries(t *testing.T) {
	h := Admin(fakeBuild(), adminDeps(t))

	rec := adminRequest(h, http.MethodPost, "/api/admin/libraries", `{"path":"Movies","type":"movies"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add: status = %d (body %q), want 201", rec.Code, rec.Body.String())
	}
	added := decode[libraryBody](t, rec)

	rec = adminRequest(h, http.MethodGet, "/api/admin/libraries", "")
	libs := decode[[]libraryBody](t, rec)
	if len(libs) != 1 || libs[0].ID != added.ID || libs[0].Path != "Movies" || libs[0].Type != "movies" ||
		libs[0].Scan.State != library.ScanQueued {
		t.Errorf("list = %+v, want Movies, scan queued", libs)
	}

	errorTests := []struct {
		name, body string
		wantStatus int
		wantCode   string
	}{
		{"overlap", `{"path":"Movies","type":"movies"}`, http.StatusConflict, errOverlap},
		{"bad type", `{"path":"TV","type":"music"}`, http.StatusBadRequest, errBadType},
		{"not a folder", `{"path":"Nope","type":"tv"}`, http.StatusBadRequest, errNotFolder},
		{"outside", `{"path":"../x","type":"tv"}`, http.StatusBadRequest, errNotFolder},
		{"bad JSON", `{`, http.StatusBadRequest, errBadRequest},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			rec := adminRequest(h, http.MethodPost, "/api/admin/libraries", tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := decode[map[string]string](t, rec)["error"]; got != tt.wantCode {
				t.Errorf("error = %q, want %q", got, tt.wantCode)
			}
		})
	}

	if rec := adminRequest(h, http.MethodDelete, "/api/admin/libraries/1", ""); rec.Code != http.StatusNoContent {
		t.Errorf("delete: status = %d, want 204", rec.Code)
	}
	if libs := decode[[]libraryBody](t, adminRequest(h, http.MethodGet, "/api/admin/libraries", "")); len(libs) != 0 {
		t.Errorf("list after delete = %+v, want empty", libs)
	}
	for _, target := range []string{"/api/admin/libraries/1", "/api/admin/libraries/x"} {
		if rec := adminRequest(h, http.MethodDelete, target, ""); rec.Code != http.StatusNotFound {
			t.Errorf("delete %s: status = %d, want 404", target, rec.Code)
		}
	}
	if rec := adminRequest(h, http.MethodPost, "/api/admin/libraries/1/scan", ""); rec.Code != http.StatusNotFound {
		t.Errorf("scan of a removed library: status = %d, want 404", rec.Code)
	}
}

func TestAdminFolders(t *testing.T) {
	h := Admin(fakeBuild(), adminDeps(t))
	tests := []struct {
		target     string
		wantStatus int
		want       string
	}{
		{"/api/admin/folders", http.StatusOK, `{"folders":["Movies","TV"]}`},
		{"/api/admin/folders?path=Movies", http.StatusOK, `{"folders":[]}`},
		{"/api/admin/folders?path=..", http.StatusBadRequest, ""},
		{"/api/admin/folders?path=Nope", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		rec := adminRequest(h, http.MethodGet, tt.target, "")
		if rec.Code != tt.wantStatus {
			t.Errorf("%s: status = %d, want %d", tt.target, rec.Code, tt.wantStatus)
		}
		if got := strings.TrimSpace(rec.Body.String()); tt.want != "" && got != tt.want {
			t.Errorf("%s: body = %s, want %s", tt.target, got, tt.want)
		}
	}
}

func TestAdminProblemsEmptyLists(t *testing.T) {
	rec := adminRequest(Admin(fakeBuild(), adminDeps(t)), http.MethodGet, "/api/admin/problems", "")
	want := `{"skipped":[],"unplayable":[],"appleOnly":[]}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestAdminJobsEmpty(t *testing.T) {
	rec := adminRequest(Admin(fakeBuild(), adminDeps(t)), http.MethodGet, "/api/admin/jobs", "")
	body := decode[struct {
		Jobs       []media.JobStatus `json:"jobs"`
		CacheBytes *int64            `json:"cacheBytes"`
		FreeBytes  uint64            `json:"freeBytes"`
	}](t, rec)
	if body.Jobs == nil || len(body.Jobs) != 0 || body.CacheBytes == nil || *body.CacheBytes != 0 || body.FreeBytes == 0 {
		t.Errorf("body = %s, want an empty jobs list, cache 0, some free space", rec.Body.String())
	}
}

func TestAdminRefusesCrossOrigin(t *testing.T) {
	h := Admin(fakeBuild(), adminDeps(t))
	req := httptest.NewRequest(http.MethodPost, "/api/admin/libraries", strings.NewReader(`{"path":"Movies","type":"movies"}`))
	req.Host = "localhost:8081"
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if rec := serve(h, req); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}
