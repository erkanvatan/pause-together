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

// create makes a browser with name, and returns its person and token. A taken name joins its holder,
// as the page does once asked.
func create(t *testing.T, s *user.Store, name string) (user.User, string) {
	t.Helper()
	u, token, err := s.Create(context.Background(), name)
	if errors.Is(err, user.ErrNameTaken) {
		_, u, token, err = s.Join(context.Background(), "", u.ID)
	}
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

// rename renames the browser's person, or, when the name is taken, joins its holder.
func rename(t *testing.T, s *user.Store, token, name string) user.User {
	t.Helper()
	_, u, err := s.Rename(context.Background(), token, name)
	if errors.Is(err, user.ErrNameTaken) {
		_, u, _, err = s.Join(context.Background(), token, u.ID)
	}
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

// A name another person has is refused with that person, and nothing changes. A free name and your
// own name in another case still work.
func TestNameTaken(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	ali, _ := create(t, s, "Ali")
	can, token := create(t, s, "Can")

	u, newToken, err := s.Create(ctx, "ALI")
	if !errors.Is(err, user.ErrNameTaken) || u != ali || newToken != "" {
		t.Errorf("Create = %+v, %q, %v; want %+v, no token, ErrNameTaken", u, newToken, err, ali)
	}
	if _, to, err := s.Rename(ctx, token, "ali"); !errors.Is(err, user.ErrNameTaken) || to != ali {
		t.Errorf("Rename = %+v, %v; want %+v, ErrNameTaken", to, err, ali)
	}
	if got := who(t, s, token); got != can {
		t.Errorf("after the refused rename: %+v, want %+v", got, can)
	}
	if n := userCount(t, s); n != 2 {
		t.Errorf("users = %d, want 2", n)
	}

	if _, to, err := s.Rename(ctx, token, "CAN"); err != nil || to.ID != can.ID || to.Name != "CAN" {
		t.Errorf("own spelling: %+v, %v; want id %d, CAN", to, err, can.ID)
	}
	if _, to, err := s.Rename(ctx, token, "Bob"); err != nil || to.ID != can.ID {
		t.Errorf("free name: %+v, %v; want id %d", to, err, can.ID)
	}
	if u, _, err := s.Create(ctx, "Carol"); err != nil || u.Name != "Carol" {
		t.Errorf("Create free name = %+v, %v", u, err)
	}
}

// Join goes by id: a person who renamed after the page asked is still the one it joins. A known browser
// keeps its token; a new or unknown one gets one.
func TestJoin(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	ali, aliToken := create(t, s, "Ali")
	can, token := create(t, s, "Can")
	bob := rename(t, s, aliToken, "Bob")

	from, to, newToken, err := s.Join(ctx, token, ali.ID)
	if err != nil || from != can || to != bob || newToken != "" {
		t.Errorf("Join = %+v, %+v, %q, %v; want %+v, %+v, no new token", from, to, newToken, err, can, bob)
	}
	if got := who(t, s, token); got != bob {
		t.Errorf("browser = %+v, want %+v", got, bob)
	}
	for _, tok := range []string{"", "AAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		from, to, newToken, err := s.Join(ctx, tok, ali.ID)
		if err != nil || from != (user.User{}) || to != bob || newToken == "" {
			t.Errorf("Join(%q) = %+v, %+v, %q, %v; want a new token for %+v", tok, from, to, newToken, err, bob)
		} else if got := who(t, s, newToken); got != bob {
			t.Errorf("new browser = %+v, want %+v", got, bob)
		}
	}
	if _, _, _, err := s.Join(ctx, token, 999); !errors.Is(err, user.ErrUnknownPerson) {
		t.Errorf("Join unknown id = %v, want ErrUnknownPerson", err)
	}
	if n := userCount(t, s); n != 2 {
		t.Errorf("users = %d, want 2", n)
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
