package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/erkanvatan/pause-together/internal/room"
)

// wsTest is a guest server with room 1 on Heat, and two users.
type wsTest struct {
	d              Deps
	srv            *httptest.Server
	alice, bob     string // tokens
	aliceID, bobID int64
	guest, admin   http.Handler
	stopHub        context.CancelFunc // stops the room loops, as a server shutdown does
}

func newWSTest(t *testing.T) *wsTest {
	t.Helper()
	d := adminDeps(t)
	hubCtx, stopHub := context.WithCancel(t.Context())
	d.Hub = room.NewHub(hubCtx, d.Rooms, testBuildID)
	t.Cleanup(d.Hub.Wait)
	if _, err := d.Libraries.DB.Exec("INSERT INTO libraries (id, path, type) VALUES (1, 'Movies', 'movies')"); err != nil {
		t.Fatal(err)
	}
	seedVideo(t, d, 1)
	one := 1
	if _, err := d.Rooms.Create(t.Context(), room.Pick{VideoID: 7, Audio: &one}); err != nil {
		t.Fatal(err)
	}
	w := &wsTest{d: d, guest: Guest(fakeBuild(), d), admin: Admin(fakeBuild(), d), stopHub: stopHub}
	for _, u := range []struct {
		name  string
		token *string
		id    *int64
	}{{"Alice", &w.alice, &w.aliceID}, {"Bob", &w.bob, &w.bobID}} {
		usr, token, err := d.Users.Create(t.Context(), u.name)
		if err != nil {
			t.Fatal(err)
		}
		*u.token, *u.id = token, usr.ID
	}
	w.srv = httptest.NewServer(w.guest)
	t.Cleanup(w.srv.Close)
	return w
}

// dial opens a socket to a room as the user with token ("" = no cookie), from origin ("" = none).
func (w *wsTest) dial(t *testing.T, roomQuery, token, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	h := http.Header{}
	if token != "" {
		h.Set("Cookie", testCookie+"="+token)
	}
	if origin != "" {
		h.Set("Origin", origin)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(w.srv.URL, "http")+"/ws?room="+roomQuery,
		&websocket.DialOptions{HTTPHeader: h})
	if c != nil {
		t.Cleanup(func() { _ = c.CloseNow() })
	}
	return c, resp, err
}

// join opens a socket to room 1, from the server's own origin, as a browser would.
func (w *wsTest) join(t *testing.T, token string) *websocket.Conn {
	t.Helper()
	c, _, err := w.dial(t, "1", token, w.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// msg is a server message, decoded loosely.
type msg map[string]any

func read(t *testing.T, c *websocket.Conn) (msg, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		return nil, err
	}
	var m msg
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return m, nil
}

// until reads messages until one of type typ passes ok (nil: any), and returns it.
func until(t *testing.T, c *websocket.Conn, typ string, ok func(msg) bool) msg {
	t.Helper()
	for {
		m, err := read(t, c)
		if err != nil {
			t.Fatalf("waiting for %s: %v", typ, err)
		}
		if m["type"] == typ && (ok == nil || ok(m)) {
			return m
		}
	}
}

func send(t *testing.T, c *websocket.Conn, v string) {
	t.Helper()
	if err := c.Write(t.Context(), websocket.MessageText, []byte(v)); err != nil {
		t.Fatal(err)
	}
}

// names returns the names in a presence list.
func names(list any) []string {
	var out []string
	for _, w := range list.([]any) {
		out = append(out, w.(map[string]any)["name"].(string))
	}
	return out
}

func TestSocketJoin(t *testing.T) {
	w := newWSTest(t)
	c := w.join(t, w.alice)

	// hello first, then the whole picture.
	var types []string
	for range 6 {
		m, err := read(t, c)
		if err != nil {
			t.Fatal(err)
		}
		types = append(types, m["type"].(string))
		switch m["type"] {
		case room.MsgHello:
			if m["buildId"] != testBuildID || m["userId"] != float64(w.aliceID) {
				t.Errorf("hello = %v", m)
			}
		case room.MsgState:
			st := m["state"].(map[string]any)
			if st["videoId"] != float64(7) || st["playing"] != false {
				t.Errorf("state = %v", st)
			}
		case room.MsgPrepare:
			// Joining opened the room, which queued its prepare. No worker runs in tests.
			if p := m["prepare"].(map[string]any); p["state"] != "queued" || p["place"] != float64(1) {
				t.Errorf("prepare = %v", p)
			}
		case room.MsgPresence:
			if got := names(m["watching"]); len(got) != 1 || got[0] != "Alice" {
				t.Errorf("watching = %v", got)
			}
		}
	}
	if want := "hello room state prepare chatHistory presence"; strings.Join(types, " ") != want {
		t.Errorf("messages = %v, want %s", types, want)
	}
}

func TestSocketPlayReachesOthers(t *testing.T) {
	w := newWSTest(t)
	a := w.join(t, w.alice)
	until(t, a, room.MsgPresence, nil)
	b := w.join(t, w.bob)
	until(t, b, room.MsgPresence, nil)

	send(t, a, `{"type":"play"}`)
	until(t, b, room.MsgState, func(m msg) bool { return m["state"].(map[string]any)["playing"] == true })

	send(t, b, `{"type":"pause"}`)
	paused := until(t, a, room.MsgPaused, nil)
	if by := paused["by"].(map[string]any); by["name"] != "Bob" {
		t.Errorf("paused by %v, want Bob", by)
	}
}

func TestSocketPing(t *testing.T) {
	w := newWSTest(t)
	c := w.join(t, w.alice)
	send(t, c, `{"type":"ping","t":1234.5}`)
	pong := until(t, c, room.MsgPong, nil)
	if pong["t"] != 1234.5 || pong["serverMs"] == nil {
		t.Errorf("pong = %v", pong)
	}
}

func TestSocketRefused(t *testing.T) {
	w := newWSTest(t)
	tests := []struct {
		name, room, token, origin string
		want                      int
	}{
		{"another origin", "1", w.alice, "http://evil.example", http.StatusForbidden},
		{"no cookie", "1", "", w.srv.URL, http.StatusUnauthorized},
		{"unknown token", "1", "nope", w.srv.URL, http.StatusUnauthorized},
		{"malformed room", "x", w.alice, w.srv.URL, http.StatusNotFound},
	}
	for _, tt := range tests {
		_, resp, err := w.dial(t, tt.room, tt.token, tt.origin)
		if err == nil {
			t.Errorf("%s: connected, want refused", tt.name)
			continue
		}
		if resp == nil || resp.StatusCode != tt.want {
			t.Errorf("%s: response %v, want status %d", tt.name, resp, tt.want)
		}
	}
}

// A room that doesn't exist (any more) is refused only after the Origin check, with a close code the
// page can read.
func TestSocketRoomGone(t *testing.T) {
	w := newWSTest(t)
	c, _, err := w.dial(t, "2", w.alice, w.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := read(t, c); websocket.CloseStatus(err) != room.CloseNotFound {
		t.Errorf("read: %v, want close %d", err, room.CloseNotFound)
	}
	if jobs := w.d.Jobs.List(); len(jobs) != 0 {
		t.Errorf("jobs = %+v, want none", jobs)
	}

	// From another origin, the room isn't even opened: its prepare isn't queued.
	if _, _, err := w.dial(t, "1", w.alice, "http://evil.example"); err == nil {
		t.Fatal("connected from another origin")
	}
	if jobs := w.d.Jobs.List(); len(jobs) != 0 {
		t.Errorf("after a cross-origin dial: jobs = %+v, want none", jobs)
	}
}

func TestSocketHeartbeat(t *testing.T) {
	w := newWSTest(t)
	w.d.Hub.Heartbeat = 300 * time.Millisecond

	// Pinging more often than that keeps the socket open.
	c := w.join(t, w.alice)
	for range 6 {
		send(t, c, `{"type":"ping","t":1}`)
		until(t, c, room.MsgPong, nil)
		time.Sleep(100 * time.Millisecond)
	}

	// Silence: the server drops the socket.
	start := time.Now()
	for {
		if _, err := read(t, c); err != nil {
			if time.Since(start) > 3*time.Second {
				t.Errorf("dropped after %v, want about 300 ms", time.Since(start))
			}
			return
		}
	}
}

func TestSocketPresence(t *testing.T) {
	w := newWSTest(t)
	a1 := w.join(t, w.alice)
	until(t, a1, room.MsgPresence, nil)
	a2 := w.join(t, w.alice)
	until(t, a2, room.MsgPresence, nil)
	b := w.join(t, w.bob)
	m := until(t, b, room.MsgPresence, nil)
	if got := strings.Join(names(m["watching"]), ","); got != "Alice,Bob" {
		t.Errorf("watching = %s, want Alice once, then Bob", got)
	}

	cards := decode[[]struct {
		Watching []room.Who `json:"watching"`
	}](t, adminRequest(w.guest, http.MethodGet, "/api/rooms", ""))
	if len(cards) != 1 || len(cards[0].Watching) != 2 || cards[0].Watching[0].Name != "Alice" {
		t.Errorf("GET /api/rooms: %+v, want Alice and Bob watching", cards)
	}

	// Alice drops both tabs: inside the reconnect grace she is still watching.
	_ = a1.CloseNow()
	_ = a2.CloseNow()
	time.Sleep(500 * time.Millisecond)
	cards = decode[[]struct {
		Watching []room.Who `json:"watching"`
	}](t, adminRequest(w.guest, http.MethodGet, "/api/rooms", ""))
	if len(cards[0].Watching) != 2 {
		t.Errorf("right after Alice dropped: watching %+v, want her still in", cards[0].Watching)
	}
}

func TestSocketArchiveAndDelete(t *testing.T) {
	w := newWSTest(t)
	a := w.join(t, w.alice)
	b := w.join(t, w.bob)
	until(t, a, room.MsgPresence, nil)
	until(t, b, room.MsgPresence, nil)

	rec := adminRequest(w.guest, http.MethodPost, "/api/rooms/1/archive", "")
	if rec.Code != http.StatusConflict || strings.TrimSpace(rec.Body.String()) != `{"error":"watching"}` {
		t.Errorf("archive while watching: %d %s, want 409 watching", rec.Code, rec.Body.String())
	}

	// A rename reaches everyone.
	adminRequest(w.guest, http.MethodPut, "/api/rooms/1/name", `{"name":"Movie night"}`)
	until(t, b, room.MsgRoom, func(m msg) bool { return m["room"].(map[string]any)["name"] == "Movie night" })

	if rec := adminRequest(w.admin, http.MethodDelete, "/api/admin/rooms/1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	for _, c := range []*websocket.Conn{a, b} {
		until(t, c, room.MsgDeleted, nil)
		_, err := read(t, c)
		if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
			t.Errorf("after deleted: %v, want a normal close", err)
		}
	}
}

// With nobody in it, a room can be archived. Then it can't play.
func TestSocketArchived(t *testing.T) {
	w := newWSTest(t)
	rec := adminRequest(w.guest, http.MethodPost, "/api/rooms/1/archive", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("archive with nobody in: %d %s", rec.Code, rec.Body.String())
	}
	// An archived room can't play: play only snaps the sender back.
	c := w.join(t, w.alice)
	until(t, c, room.MsgPresence, nil)
	send(t, c, `{"type":"play"}`)
	m := until(t, c, room.MsgState, nil)
	if m["state"].(map[string]any)["playing"] != false {
		t.Errorf("archived room plays: %v", m)
	}
}

// chatText returns a chat message's text, and its id.
func chatText(m msg) (string, float64) {
	c := m["message"].(map[string]any)
	return c["text"].(string), c["id"].(float64)
}

// A message reaches everyone in the room, with the room's video and position. Only its sender can
// delete it, and a new joiner gets the chat so far.
func TestSocketChat(t *testing.T) {
	w := newWSTest(t)
	a := w.join(t, w.alice)
	until(t, a, room.MsgPresence, nil)
	b := w.join(t, w.bob)
	until(t, b, room.MsgPresence, nil)

	send(t, a, `{"type":"seek","positionMs":90000}`)
	send(t, a, `{"type":"chat","text":"Hello","replyTo":null}`)
	var hello float64
	for _, c := range []*websocket.Conn{a, b} {
		m := until(t, c, room.MsgChat, nil)["message"].(map[string]any)
		if m["text"] != "Hello" || m["from"].(map[string]any)["name"] != "Alice" ||
			m["video"].(map[string]any)["id"] != float64(7) || m["positionMs"] != float64(90000) || m["replyTo"] != nil {
			t.Errorf("chat = %v", m)
		}
		hello = m["id"].(float64)
	}

	send(t, b, fmt.Sprintf(`{"type":"chat","text":"Hi","replyTo":%v}`, hello))
	reply := until(t, a, room.MsgChat, nil)["message"].(map[string]any)
	if q, _ := reply["replyTo"].(map[string]any); q == nil || q["text"] != "Hello" {
		t.Errorf("reply = %v, want it to quote Hello", reply)
	}

	// Bob can't delete Alice's message. His next message shows the loop is past his delete.
	send(t, b, fmt.Sprintf(`{"type":"deleteChat","id":%v}`, hello))
	send(t, b, `{"type":"chat","text":"after","replyTo":null}`)
	for {
		m, err := read(t, a)
		if err != nil {
			t.Fatal(err)
		}
		if m["type"] == room.MsgChatDeleted {
			t.Fatal("Bob deleted Alice's message")
		}
		if m["type"] == room.MsgChat {
			if text, _ := chatText(m); text == "after" {
				break
			}
		}
	}

	send(t, a, fmt.Sprintf(`{"type":"deleteChat","id":%v}`, hello))
	for _, c := range []*websocket.Conn{a, b} {
		if m := until(t, c, room.MsgChatDeleted, nil); m["id"] != hello {
			t.Errorf("chatDeleted = %v, want id %v", m, hello)
		}
	}

	// Alice's second tab gets the chat without the deleted message. The reply now quotes it as deleted.
	c := w.join(t, w.alice)
	history := until(t, c, room.MsgChatHistory, nil)["messages"].([]any)
	var texts []string
	for _, h := range history {
		texts = append(texts, h.(map[string]any)["text"].(string))
	}
	if strings.Join(texts, ",") != "Hi,after" {
		t.Errorf("history = %v, want Hi,after", texts)
	}
	if q := history[0].(map[string]any)["replyTo"].(map[string]any); q["deleted"] != true || q["text"] != "" {
		t.Errorf("reply quotes %v, want a deleted message", q)
	}
}

// The longest message fits the socket's read limit, even with JSON escaping every character: a
// control character takes 6 bytes, like \u0001.
func TestSocketLongChat(t *testing.T) {
	w := newWSTest(t)
	a := w.join(t, w.alice)
	until(t, a, room.MsgPresence, nil)
	send(t, a, `{"type":"chat","text":"`+strings.Repeat(`\u0001`, room.MaxMessageRunes)+`","replyTo":null}`)
	if text, _ := chatText(until(t, a, room.MsgChat, nil)); text != strings.Repeat("\x01", room.MaxMessageRunes) {
		t.Errorf("got %d runes back, want the %d sent", len([]rune(text)), room.MaxMessageRunes)
	}
}

// A shutdown pauses a playing room, tells the pages, and saves where it is, not where it was at its
// last timed save. A page that played on would jump back when the server returns.
func TestSocketShutdownPausesAndSaves(t *testing.T) {
	w := newWSTest(t)
	a := w.join(t, w.alice)
	until(t, a, room.MsgPresence, nil)
	send(t, a, `{"type":"play"}`)
	until(t, a, room.MsgState, func(m msg) bool { return m["state"].(map[string]any)["playing"] == true })
	time.Sleep(300 * time.Millisecond) // well under SaveEveryMs

	w.stopHub()
	w.d.Hub.Wait()
	var pos int64
	if err := w.d.Rooms.DB.QueryRow("SELECT position_ms FROM rooms WHERE id = 1").Scan(&pos); err != nil {
		t.Fatal(err)
	}
	if pos < 300 {
		t.Errorf("saved position = %d ms, want at least 300", pos)
	}
	var paused bool
	for {
		m, err := read(t, a)
		if err != nil {
			if code := websocket.CloseStatus(err); code != websocket.StatusGoingAway {
				t.Errorf("socket closed with %v, want going away", err)
			}
			break
		}
		if m["type"] == room.MsgState {
			st := m["state"].(map[string]any)
			paused = st["playing"] == false && int64(st["positionMs"].(float64)) == pos
		}
	}
	if !paused {
		t.Errorf("no paused state at %d ms before the socket closed", pos)
	}
}
