package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
)

// User is a visitor who picked a name.
type User struct {
	ID   int64
	Name string
}

// Store reads and writes users. Names passed in must already be cleaned with CleanName.
type Store struct {
	DB *sql.DB
}

// Create makes a user and returns it with its token. Only the token's hash is stored.
func (s *Store) Create(ctx context.Context, name string) (User, string, error) {
	token := rand.Text()
	hash := sha256.Sum256([]byte(token))
	var id int64
	err := s.DB.QueryRowContext(ctx,
		"INSERT INTO users (token_hash, name) VALUES (?, ?) RETURNING id", hash[:], name,
	).Scan(&id)
	if err != nil {
		return User{}, "", err
	}
	return User{ID: id, Name: name}, token, nil
}

// ByToken finds the user a token belongs to. An unknown token reports false, not an error.
func (s *Store) ByToken(ctx context.Context, token string) (User, bool, error) {
	hash := sha256.Sum256([]byte(token))
	u := User{}
	err := s.DB.QueryRowContext(ctx,
		"SELECT id, name FROM users WHERE token_hash = ?", hash[:],
	).Scan(&u.ID, &u.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return u, true, nil
}

// Rename sets a user's name.
func (s *Store) Rename(ctx context.Context, id int64, name string) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE users SET name = ? WHERE id = ?", name, id)
	return err
}
