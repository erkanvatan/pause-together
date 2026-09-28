package room

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/erkanvatan/pause-together/internal/library"
)

// TestProtocol checks the shared fixture (web/src/lib/protocol.test.ts reads it too): every client
// message decodes to the Go value we expect, and every server message is what Go encodes.
func TestProtocol(t *testing.T) {
	b, err := os.ReadFile("testdata/protocol.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Client []json.RawMessage `json:"client"`
		Server []json.RawMessage `json:"server"`
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}

	sidecar := int64(7)
	client := []ClientMsg{
		{Type: MsgPing, T: 1234.5},
		{Type: MsgPlay},
		{Type: MsgPause},
		{Type: MsgSeek, PositionMs: 90_000},
		{Type: MsgPlayAnyway},
		{Type: MsgSubtitle, Subtitle: &Subtitle{Stream: stream(3)}},
		{Type: MsgSubtitle, Subtitle: &Subtitle{Sidecar: &sidecar}},
		{Type: MsgSubtitle},
		{Type: MsgOffset, Ms: -1500},
		{Type: MsgStatus, Status: "ready", PositionMs: 12_345.678},
		{Type: MsgStatus, Status: "buffering"},
		{Type: MsgStatus, Status: "away", PositionMs: 5000},
		{Type: MsgStatus, Status: "cantPlay"},
	}
	if len(fixture.Client) != len(client) {
		t.Fatalf("fixture has %d client messages, want %d", len(fixture.Client), len(client))
	}
	for i, raw := range fixture.Client {
		var got ClientMsg
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("client %d: %v", i, err)
		}
		if !reflect.DeepEqual(got, client[i]) {
			t.Errorf("client %d: %s decodes to %+v, want %+v", i, raw, got, client[i])
		}
		if got.Status != "" && statusNames[got.Status] == 0 {
			t.Errorf("client %d: unknown status %q", i, got.Status)
		}
	}

	heat := Room{
		ID:   1,
		Name: "Movie night",
		Video: library.VideoRef{VideoSummary: library.VideoSummary{ID: 7, Type: "movies", Title: "Heat", Year: 1995,
			DurationMs: 3_600_000, CodecString: "avc1.640028"}},
		Subtitle: &Subtitle{Sidecar: &sidecar},
	}
	server := []any{
		HelloMsg{Type: MsgHello, BuildID: "1727000000000", UserID: 1},
		StateMsg{Type: MsgState, State: State{VideoID: 7, Audio: stream(1), Subtitle: &Subtitle{Stream: stream(3)},
			SubtitleOffsetMs: 500, DurationMs: 3_600_000, Playing: true, PositionMs: 90_000, AtMs: 4000,
			Waiting: []Who{alice}, Behind: []Lag{{Who: bob, Ms: 3000}}}},
		RoomMsg{Type: MsgRoom, Room: heat},
		PresenceMsg{Type: MsgPresence, Watching: []Who{alice}, WasHere: []Who{}},
		PrepareMsg{Type: MsgPrepare, Prepare: Prepare{State: "queued", Place: 2}},
		PrepareMsg{Type: MsgPrepare, Prepare: Prepare{State: "ready", Key: "0123456789abcdef0123456789abcdef"}},
		PausedMsg{Type: MsgPaused, By: bob},
		PongMsg{Type: MsgPong, T: 1234.5, ServerMs: 98_765},
		TypeMsg{Type: MsgDeleted},
	}
	if len(fixture.Server) != len(server) {
		t.Fatalf("fixture has %d server messages, want %d", len(fixture.Server), len(server))
	}
	for i, raw := range fixture.Server {
		enc, err := json.Marshal(server[i])
		if err != nil {
			t.Fatal(err)
		}
		var got, want any
		if err := json.Unmarshal(enc, &got); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("server %d: Go encodes %s\nfixture has %s", i, enc, raw)
		}
	}
}
