package api

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const (
	indexBody = "<!doctype html><title>index</title>"
	jsBody    = "console.log('app')"

	noCache   = "no-cache"
	immutable = "public, max-age=31536000, immutable"
)

// fakeBuild stands in for the SvelteKit build output.
func fakeBuild() fstest.MapFS {
	return fstest.MapFS{
		"index.html":                      {Data: []byte(indexBody)},
		"favicon.png":                     {Data: []byte("png")},
		"_app/version.json":               {Data: []byte(`{"version":"1"}`)},
		"_app/immutable/entry/app.abc.js": {Data: []byte(jsBody)},
	}
}

// handlers returns both listeners' handlers, keyed by name, over the same fake build and database.
func handlers(t *testing.T) map[string]http.Handler {
	build := fakeBuild()
	users := testUsers(t)
	return map[string]http.Handler{
		"guest": Guest(build, deps(users)),
		"admin": Admin(build, deps(users)),
	}
}

// do sends a request with a Host the admin listener accepts, so both handlers can be tested alike.
func do(h http.Handler, method, target string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = "localhost:8081"
	maps.Copy(req.Header, header)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRoutes(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantBody    string // exact body; empty means don't check
		wantCache   string // exact Cache-Control; empty means don't check
		wantTypePfx string // Content-Type prefix; empty means don't check
	}{
		{name: "health", path: "/api/health", wantStatus: 200, wantTypePfx: "application/json"},
		{name: "unknown api", path: "/api/nope", wantStatus: 404},
		{name: "unknown admin api", path: "/api/admin/nope", wantStatus: 404},
		{name: "unknown stream", path: "/stream/nope", wantStatus: 404},
		{name: "root", path: "/", wantStatus: 200, wantBody: indexBody, wantCache: noCache},
		{name: "app route", path: "/rooms/abc", wantStatus: 200, wantBody: indexBody, wantCache: noCache},
		{name: "index by name", path: "/index.html", wantStatus: 200, wantBody: indexBody, wantCache: noCache},
		{name: "directory falls back", path: "/_app", wantStatus: 200, wantBody: indexBody, wantCache: noCache},
		{name: "directory with slash falls back", path: "/_app/immutable/", wantStatus: 200, wantBody: indexBody, wantCache: noCache},
		{
			name: "immutable asset", path: "/_app/immutable/entry/app.abc.js",
			wantStatus: 200, wantBody: jsBody, wantCache: immutable, wantTypePfx: "text/javascript",
		},
		{name: "mutable _app file", path: "/_app/version.json", wantStatus: 200, wantCache: noCache},
		{name: "plain static file", path: "/favicon.png", wantStatus: 200, wantBody: "png", wantCache: noCache},
	}

	for hName, h := range handlers(t) {
		for _, tt := range tests {
			t.Run(hName+"/"+tt.name, func(t *testing.T) {
				rec := do(h, http.MethodGet, tt.path, nil)

				if rec.Code != tt.wantStatus {
					t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
				}
				if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
					t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
				}
				if got := rec.Header().Get("Cache-Control"); tt.wantCache != "" && got != tt.wantCache {
					t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
				}
				if got := rec.Header().Get("Content-Type"); tt.wantTypePfx != "" && !strings.HasPrefix(got, tt.wantTypePfx) {
					t.Errorf("Content-Type = %q, want prefix %q", got, tt.wantTypePfx)
				}
			})
		}
	}
}

func TestSPARejectsWrites(t *testing.T) {
	for hName, h := range handlers(t) {
		t.Run(hName, func(t *testing.T) {
			// Same-origin, so CrossOriginProtection lets it through and the SPA handler decides.
			header := http.Header{"Sec-Fetch-Site": {"same-origin"}}
			if rec := do(h, http.MethodPost, "/rooms/abc", header); rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("POST /rooms/abc status = %d, want 405", rec.Code)
			}
		})
	}
}

func TestAdminHostCheck(t *testing.T) {
	tests := []struct {
		host string
		want int
	}{
		{"localhost:8081", 200},
		{"localhost", 200},
		{"LOCALHOST:8081", 200},
		{"127.0.0.1", 200},
		{"127.0.0.1:8081", 200},
		{"[::1]", 200},
		{"[::1]:8081", 200},
		{"evil.com", 403},
		{"evil.com:8081", 403},
		{"localhost.evil.com", 403},
		{"127.0.0.1.nip.io", 403},
		{"100.64.0.1:8081", 403}, // a Tailscale IP: guests never reach the admin API
		{"", 403},
	}

	h := Admin(fakeBuild(), deps(testUsers(t)))
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			req.Host = tt.host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("Host %q: status = %d, want %d", tt.host, rec.Code, tt.want)
			}
		})
	}
}

func TestGuestIgnoresHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Host = "evil.com"
	rec := httptest.NewRecorder()
	Guest(fakeBuild(), deps(testUsers(t))).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestCrossOriginPostRefused(t *testing.T) {
	for hName, h := range handlers(t) {
		t.Run(hName, func(t *testing.T) {
			header := http.Header{"Sec-Fetch-Site": {"cross-site"}}
			if rec := do(h, http.MethodPost, "/api/health", header); rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
		})
	}
}
