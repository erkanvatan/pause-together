package user

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/erkanvatan/pause-together/internal/store"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), store.Migrations())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Store{DB: db}
}

func TestCreateAndFind(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	u, token, err := s.Create(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.ByToken(ctx, token)
	if err != nil || !ok {
		t.Fatalf("ByToken = %v, %v; want found", ok, err)
	}
	if got != u || got.Name != "Alice" {
		t.Errorf("ByToken = %+v, want %+v", got, u)
	}

	_, token2, err := s.Create(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if token2 == token {
		t.Error("two users got the same token")
	}
}

func TestUnknownToken(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	if _, _, err := s.Create(ctx, "Alice"); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "nope", "AAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if _, ok, err := s.ByToken(ctx, token); ok || err != nil {
			t.Errorf("ByToken(%q) = %v, %v; want not found", token, ok, err)
		}
	}
}

func TestRename(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	u, token, err := s.Create(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Rename(ctx, u.ID, "Bob"); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.ByToken(ctx, token)
	if err != nil || got.Name != "Bob" {
		t.Errorf("after rename: %+v, %v; want Bob", got, err)
	}
}

func TestTokenStoredHashed(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	_, token, err := s.Create(ctx, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err := s.DB.QueryRowContext(ctx, "SELECT token_hash FROM users").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte(token)) {
		t.Error("raw token found in the database")
	}
}
