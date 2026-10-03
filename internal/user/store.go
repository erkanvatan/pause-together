package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
)

// User is a person: everyone who picked one name (NameKey), on any number of browsers.
type User struct {
	ID   int64
	Name string
}

// Store reads and writes users and their tokens. Names passed in must already be cleaned with CleanName.
type Store struct {
	DB *sql.DB
}

// Create gives a new browser a token, and makes it the person with this name, new or not. Only the
// token's hash is stored.
func (s *Store) Create(ctx context.Context, name string) (User, string, error) {
	u, err := s.person(ctx, name)
	if err != nil {
		return User{}, "", err
	}
	token := rand.Text()
	hash := sha256.Sum256([]byte(token))
	if _, err := s.DB.ExecContext(ctx,
		"INSERT INTO tokens (token_hash, user_id) VALUES (?, ?)", hash[:], u.ID,
	); err != nil {
		return User{}, "", err
	}
	return u, token, nil
}

// ByToken finds the user a token belongs to. An unknown token reports false, not an error.
func (s *Store) ByToken(ctx context.Context, token string) (User, bool, error) {
	hash := sha256.Sum256([]byte(token))
	u := User{}
	err := s.DB.QueryRowContext(ctx,
		"SELECT u.id, u.name FROM tokens t JOIN users u ON u.id = t.user_id WHERE t.token_hash = ?", hash[:],
	).Scan(&u.ID, &u.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return u, true, nil
}

// Rename moves the browser with this token, now the person from, to the person with this name, new
// or not. The person it leaves keeps its name and its other browsers. The same name in another case is
// no move: it changes the spelling for every browser of the person, the only way to fix one.
func (s *Store) Rename(ctx context.Context, token string, from User, name string) (User, error) {
	if NameKey(name) == NameKey(from.Name) {
		_, err := s.DB.ExecContext(ctx, "UPDATE users SET name = ? WHERE id = ?", name, from.ID)
		return User{ID: from.ID, Name: name}, err
	}
	u, err := s.person(ctx, name)
	if err != nil {
		return User{}, err
	}
	hash := sha256.Sum256([]byte(token))
	_, err = s.DB.ExecContext(ctx, "UPDATE tokens SET user_id = ? WHERE token_hash = ?", u.ID, hash[:])
	return u, err
}

// person finds the person with this name, or makes them. A found person keeps their own spelling.
// If the caller fails after this, all that's left is a person with no browser, as after a rename.
func (s *Store) person(ctx context.Context, name string) (User, error) {
	u := User{}
	// A no-op update rather than DO NOTHING: that returns no row when the name is taken.
	err := s.DB.QueryRowContext(ctx, `INSERT INTO users (name, name_key) VALUES (?, ?)
		ON CONFLICT (name_key) DO UPDATE SET name = name RETURNING id, name`, name, NameKey(name),
	).Scan(&u.ID, &u.Name)
	return u, err
}
