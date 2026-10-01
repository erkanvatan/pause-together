package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erkanvatan/pause-together/internal/store"
	"github.com/erkanvatan/pause-together/internal/user"
)

func testUsers(t *testing.T) *user.Store {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), store.Migrations())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &user.Store{DB: db}
}

// testCookie isn't the prod default, so a hardcoded cookie name would fail the tests.
const testCookie = "pt_test"

func deps(users *user.Store) Deps {
	return Deps{Users: users, TokenCookie: testCookie}
}

type meBody struct {
	Name     *string `json:"name"`
	IsAdmin  bool    `json:"isAdmin"`
	GuestURL string  `json:"guestUrl"`
}

// meRequest builds a request to /api/me that passes the Host check and CrossOriginProtection.
func meRequest(method, body, token string) *http.Request {
	req := httptest.NewRequest(method, "/api/me", strings.NewReader(body))
	req.Host = "localhost:8081"
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: testCookie, Value: token})
	}
	return req
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeMe(t *testing.T, rec *httptest.ResponseRecorder) meBody {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	var b meBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return b
}

// tokenFrom returns the token cookie a response set, or nil.
func tokenFrom(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == testCookie {
			return c
		}
	}
	return nil
}

func userCount(t *testing.T, users *user.Store) int {
	t.Helper()
	var n int
	if err := users.DB.QueryRow("SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func checkCookie(t *testing.T, c *http.Cookie) {
	t.Helper()
	if c == nil {
		t.Fatal("no token cookie set")
	}
	if !c.HttpOnly {
		t.Error("cookie not HttpOnly")
	}
	// Guests use plain HTTP: a Secure cookie would be dropped by their browser.
	if c.Secure {
		t.Error("cookie is Secure")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q, want /", c.Path)
	}
	if c.MaxAge != 400*24*60*60 {
		t.Errorf("MaxAge = %d, want %d", c.MaxAge, 400*24*60*60)
	}
}

func TestMeNewVisitor(t *testing.T) {
	h := Guest(fakeBuild(), deps(testUsers(t)))
	rec := serve(h, meRequest(http.MethodGet, "", ""))
	if b := decodeMe(t, rec); b.Name != nil {
		t.Errorf("name = %q, want null", *b.Name)
	}
	if c := tokenFrom(rec); c != nil {
		t.Errorf("cookie set for a visitor without a name: %v", c)
	}
}

func TestMeCreateThenReturn(t *testing.T) {
	h := Guest(fakeBuild(), deps(testUsers(t)))

	rec := serve(h, meRequest(http.MethodPost, `{"name":"  Şükrü  "}`, ""))
	if b := decodeMe(t, rec); b.Name == nil || *b.Name != "Şükrü" {
		t.Errorf("POST name = %v, want Şükrü", b.Name)
	}
	created := tokenFrom(rec)
	checkCookie(t, created)

	// Next visit: same name, cookie refreshed with the same token.
	rec = serve(h, meRequest(http.MethodGet, "", created.Value))
	if b := decodeMe(t, rec); b.Name == nil || *b.Name != "Şükrü" {
		t.Errorf("GET name = %v, want Şükrü", b.Name)
	}
	refreshed := tokenFrom(rec)
	checkCookie(t, refreshed)
	if refreshed.Value != created.Value {
		t.Error("refresh changed the token")
	}
}

func TestMeRenameKeepsToken(t *testing.T) {
	users := testUsers(t)
	h := Guest(fakeBuild(), deps(users))
	token := tokenFrom(serve(h, meRequest(http.MethodPost, `{"name":"Alice"}`, ""))).Value

	rec := serve(h, meRequest(http.MethodPost, `{"name":"Bob"}`, token))
	if b := decodeMe(t, rec); b.Name == nil || *b.Name != "Bob" {
		t.Errorf("name = %v, want Bob", b.Name)
	}
	if c := tokenFrom(rec); c == nil || c.Value != token {
		t.Errorf("rename cookie = %v, want the same token", c)
	}
	if n := userCount(t, users); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
}

func TestMeUnknownTokenIsNewVisitor(t *testing.T) {
	users := testUsers(t)
	h := Guest(fakeBuild(), deps(users))
	const unknown = "AAAAAAAAAAAAAAAAAAAAAAAAAA"

	if b := decodeMe(t, serve(h, meRequest(http.MethodGet, "", unknown))); b.Name != nil {
		t.Errorf("name = %q, want null", *b.Name)
	}

	rec := serve(h, meRequest(http.MethodPost, `{"name":"Alice"}`, unknown))
	decodeMe(t, rec)
	if c := tokenFrom(rec); c == nil || c.Value == unknown {
		t.Errorf("cookie = %v, want a new token", c)
	}
	if n := userCount(t, users); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
}

func TestMeBadInput(t *testing.T) {
	bodies := []string{
		`{"name":"   "}`,
		`{"name":"a\tb"}`,
		`{"name":"` + strings.Repeat("a", 33) + `"}`,
		`{"name":42}`,
		`not json`,
		``,
		`{"name":"` + strings.Repeat("a", 5000) + `"}`, // over the body limit
	}
	users := testUsers(t)
	h := Guest(fakeBuild(), deps(users))
	for _, body := range bodies {
		rec := serve(h, meRequest(http.MethodPost, body, ""))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %.40q: status = %d, want 400", body, rec.Code)
		}
		if c := tokenFrom(rec); c != nil {
			t.Errorf("body %.40q: cookie set", body)
		}
	}
	if n := userCount(t, users); n != 0 {
		t.Errorf("users = %d, want 0", n)
	}
}

func TestMeIsAdmin(t *testing.T) {
	users := testUsers(t)
	build := fakeBuild()
	token := tokenFrom(serve(Guest(build, deps(users)), meRequest(http.MethodPost, `{"name":"Alice"}`, ""))).Value

	tests := []struct {
		name string
		h    http.Handler
		want bool
	}{
		{"guest", Guest(build, deps(users)), false},
		{"admin", Admin(build, deps(users)), true},
	}
	for _, tt := range tests {
		for _, tok := range []string{"", token} {
			if b := decodeMe(t, serve(tt.h, meRequest(http.MethodGet, "", tok))); b.IsAdmin != tt.want {
				t.Errorf("%s (token %q): isAdmin = %v, want %v", tt.name, tok, b.IsAdmin, tt.want)
			}
		}
	}
}

// Only the host is shown the guest link.
func TestMeGuestURL(t *testing.T) {
	users := testUsers(t)
	build := fakeBuild()
	d := deps(users)
	d.GuestURL = "http://100.101.102.103:8420"
	tests := []struct {
		name string
		h    http.Handler
		want string
	}{
		{"guest", Guest(build, d), ""},
		{"admin", Admin(build, d), d.GuestURL},
	}
	for _, tt := range tests {
		if b := decodeMe(t, serve(tt.h, meRequest(http.MethodGet, "", ""))); b.GuestURL != tt.want {
			t.Errorf("%s: guestUrl = %q, want %q", tt.name, b.GuestURL, tt.want)
		}
	}
}

func TestMeCrossOrigin(t *testing.T) {
	// The Host each listener really sees: guests use the Tailscale IP, the host uses localhost.
	hosts := map[string]string{"guest": "100.64.0.1:8420", "admin": "localhost:8421"}
	tests := []struct {
		name   string
		origin string // "self" means http:// + the request's Host
		fetch  string // Sec-Fetch-Site; empty means not sent
		want   int
	}{
		{name: "cross-site fetch metadata", origin: "http://evil.com", fetch: "cross-site", want: http.StatusForbidden},
		// Plain HTTP pages get no Sec-Fetch-Site, so the Origin check is all there is.
		{name: "foreign Origin, no fetch metadata", origin: "http://evil.com", want: http.StatusForbidden},
		{name: "same origin, no fetch metadata", origin: "self", want: http.StatusOK},
	}
	for hName, h := range handlers(t) {
		for _, tt := range tests {
			t.Run(hName+"/"+tt.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "/api/me", strings.NewReader(`{"name":"Alice"}`))
				req.Host = hosts[hName]
				origin := tt.origin
				if origin == "self" {
					origin = "http://" + req.Host
				}
				req.Header.Set("Origin", origin)
				if tt.fetch != "" {
					req.Header.Set("Sec-Fetch-Site", tt.fetch)
				}
				if rec := serve(h, req); rec.Code != tt.want {
					t.Errorf("status = %d, want %d", rec.Code, tt.want)
				}
			})
		}
	}
}
