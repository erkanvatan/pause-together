// An outside test package: store imports user for the name key, so a test inside user can't import store.
package user_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/erkanvatan/pause-together/internal/store"
	"github.com/erkanvatan/pause-together/internal/user"
)

func testStore(t *testing.T) *user.Store {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), store.Migrations())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &user.Store{DB: db}
}

// create makes a browser with name, and returns its person and token.
func create(t *testing.T, s *user.Store, name string) (user.User, string) {
	t.Helper()
	u, token, err := s.Create(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return u, token
}

// who returns the user a token belongs to.
func who(t *testing.T, s *user.Store, token string) user.User {
	t.Helper()
	u, ok, err := s.ByToken(context.Background(), token)
	if err != nil || !ok {
		t.Fatalf("ByToken = %v, %v; want found", ok, err)
	}
	return u
}

func rename(t *testing.T, s *user.Store, token, name string) user.User {
	t.Helper()
	_, u, err := s.Rename(context.Background(), token, name)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func userCount(t *testing.T, s *user.Store) int {
	t.Helper()
	var n int
	if err := s.DB.QueryRow("SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateAndFind(t *testing.T) {
	s := testStore(t)
	u, token := create(t, s, "Alice")
	if got := who(t, s, token); got != u || got.Name != "Alice" {
		t.Errorf("ByToken = %+v, want %+v", got, u)
	}
	_, token2 := create(t, s, "Bob")
	if token2 == token {
		t.Error("two browsers got the same token")
	}
}

// Two browsers that pick the same name, in any case, are one person, with the first spelling.
func TestSameNameIsOnePerson(t *testing.T) {
	s := testStore(t)
	ali, phone := create(t, s, "Ali")
	other, laptop := create(t, s, "ali")
	if other != ali {
		t.Errorf("second browser = %+v, want %+v", other, ali)
	}
	if phone == laptop {
		t.Error("two browsers got the same token")
	}
	for _, token := range []string{phone, laptop} {
		if got := who(t, s, token); got != ali {
			t.Errorf("ByToken = %+v, want %+v", got, ali)
		}
	}
	if n := userCount(t, s); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
}

// Logging out forgets the browser's token. The person stays, with its other browsers.
func TestForget(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	ali, phone := create(t, s, "Ali")
	_, laptop := create(t, s, "Ali")
	if forgot, err := s.Forget(ctx, phone); !forgot || err != nil {
		t.Fatalf("Forget = %v, %v; want true", forgot, err)
	}
	if forgot, err := s.Forget(ctx, phone); forgot || err != nil {
		t.Errorf("Forget again = %v, %v; want false", forgot, err)
	}
	if _, ok, err := s.ByToken(ctx, phone); ok || err != nil {
		t.Errorf("phone after Forget = %v, %v; want not found", ok, err)
	}
	if got := who(t, s, laptop); got != ali {
		t.Errorf("laptop = %+v, want %+v", got, ali)
	}
}

// A rename by a browser that logged out meanwhile says so, and changes nobody.
func TestRenameUnknownToken(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	ali, token := create(t, s, "Ali")
	if _, err := s.Forget(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Rename(ctx, token, "Bo"); !errors.Is(err, user.ErrUnknownToken) {
		t.Errorf("Rename = %v, want ErrUnknownToken", err)
	}
	var name string
	if err := s.DB.QueryRow("SELECT name FROM users WHERE id = ?", ali.ID).Scan(&name); err != nil || name != "Ali" {
		t.Errorf("Ali = %q, %v; want unchanged", name, err)
	}
}

func TestUnknownToken(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	create(t, s, "Alice")
	for _, token := range []string{"", "nope", "AAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if _, ok, err := s.ByToken(ctx, token); ok || err != nil {
			t.Errorf("ByToken(%q) = %v, %v; want not found", token, ok, err)
		}
	}
}

// A rename to a free name renames the person, for every browser of theirs. No new person is made.
func TestRenameToNewName(t *testing.T) {
	s := testStore(t)
	ali, phone := create(t, s, "Ali")
	_, laptop := create(t, s, "Ali")

	bo := user.User{ID: ali.ID, Name: "Bo"}
	if got := rename(t, s, laptop, "Bo"); got != bo {
		t.Errorf("renamed = %+v, want %+v", got, bo)
	}
	for _, token := range []string{phone, laptop} {
		if got := who(t, s, token); got != bo {
			t.Errorf("ByToken = %+v, want %+v", got, bo)
		}
	}
	if n := userCount(t, s); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
	// The old name is free again: picking it makes a new person.
	if got, _ := create(t, s, "Ali"); got.ID == ali.ID {
		t.Errorf("Ali after the rename = %+v, want a new person", got)
	}
}

// A rename to a taken name moves only this browser to that person, with its spelling. The person it
// leaves keeps its name and its other browsers.
func TestRenameToTakenName(t *testing.T) {
	s := testStore(t)
	ali, _ := create(t, s, "Ali")
	can, phone := create(t, s, "Can")
	_, laptop := create(t, s, "Can")
	if got := rename(t, s, laptop, "ALI"); got != ali {
		t.Errorf("renamed = %+v, want %+v", got, ali)
	}
	if got := who(t, s, laptop); got != ali {
		t.Errorf("laptop = %+v, want %+v", got, ali)
	}
	if got := who(t, s, phone); got != can {
		t.Errorf("phone = %+v, want %+v", got, can)
	}
}

// A rename to your own name in another case fixes the spelling for every browser.
func TestRenameSpelling(t *testing.T) {
	s := testStore(t)
	mom, phone := create(t, s, "mom")
	_, laptop := create(t, s, "mom")
	if got := rename(t, s, phone, "Mom"); got.ID != mom.ID || got.Name != "Mom" {
		t.Errorf("renamed = %+v, want id %d, Mom", got, mom.ID)
	}
	for _, token := range []string{phone, laptop} {
		if got := who(t, s, token); got.ID != mom.ID || got.Name != "Mom" {
			t.Errorf("ByToken = %+v, want id %d, Mom", got, mom.ID)
		}
	}
}

// A person whose last browser picks another person's name stays: their messages still show it.
func TestPersonWithNoBrowserStays(t *testing.T) {
	s := testStore(t)
	ali, token := create(t, s, "Ali")
	create(t, s, "Bo")
	rename(t, s, token, "Bo")
	var name string
	if err := s.DB.QueryRow("SELECT name FROM users WHERE id = ?", ali.ID).Scan(&name); err != nil || name != "Ali" {
		t.Errorf("old person = %q, %v; want Ali", name, err)
	}
	// Picking the name again makes this browser that person once more.
	if got := rename(t, s, token, "ali"); got != ali {
		t.Errorf("back = %+v, want %+v", got, ali)
	}
}

func TestTokenStoredHashed(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	_, token := create(t, s, "Alice")
	var stored []byte
	if err := s.DB.QueryRowContext(ctx, "SELECT token_hash FROM tokens").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte(token)) {
		t.Error("raw token found in the database")
	}
}
