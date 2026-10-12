package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
)

// User is a person: one name (NameKey), on any number of browsers.
type User struct {
	ID   int64
	Name string
}

// ErrUnknownToken means no browser has the token: it logged out, or never had a name.
var ErrUnknownToken = errors.New("unknown token")

// Store reads and writes users and their tokens. Names passed in must already be cleaned with CleanName.
type Store struct {
	DB *sql.DB
}

// Create gives a new browser a token, and makes it the person with this name, new or not. Only the
// token's hash is stored.
func (s *Store) Create(ctx context.Context, name string) (User, string, error) {
	u, err := person(ctx, s.DB, name)
	if err != nil {
		return User{}, "", err
	}
	token := rand.Text()
	if _, err := s.DB.ExecContext(ctx,
		"INSERT INTO tokens (token_hash, user_id) VALUES (?, ?)", TokenHash(token), u.ID,
	); err != nil {
		return User{}, "", err
	}
	return u, token, nil
}

// ByToken finds the user a token belongs to. An unknown token reports false, not an error.
func (s *Store) ByToken(ctx context.Context, token string) (User, bool, error) {
	u, err := byToken(ctx, s.DB, token)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return u, true, nil
}

// Rename gives the person whose browser has this token a new name, and returns who they were and who
// they are now. When no other person has the name, the person is renamed: every browser of theirs and
// every message shows it. The same name in another case is a rename too, the only way to fix a
// spelling. When the name is another person's, only this browser moves to them, and the person it
// leaves keeps its name and its other browsers. An unknown token is ErrUnknownToken.
func (s *Store) Rename(ctx context.Context, token string, name string) (from, to User, err error) {
	// One transaction (immediate, see store.Open), so neither the browser's person nor the name's holder
	// can change between the reads and the writes.
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, User{}, err
	}
	defer func() { _ = tx.Rollback() }()
	from, err = byToken(ctx, tx, token)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, User{}, ErrUnknownToken
	}
	if err != nil {
		return User{}, User{}, err
	}
	key := NameKey(name)
	res, err := tx.ExecContext(ctx, `UPDATE users SET name = ?, name_key = ? WHERE id = ?
		AND NOT EXISTS (SELECT 1 FROM users WHERE name_key = ? AND id <> ?)`, name, key, from.ID, key, from.ID)
	if err != nil {
		return User{}, User{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return User{}, User{}, err
	}
	to = User{ID: from.ID, Name: name}
	if n == 0 {
		if to, err = person(ctx, tx, name); err != nil {
			return User{}, User{}, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE tokens SET user_id = ? WHERE token_hash = ?", to.ID, TokenHash(token)); err != nil {
			return User{}, User{}, err
		}
	}
	return from, to, tx.Commit()
}

// Forget deletes a browser's token: it logs out. Its person stays, with their messages. It reports
// whether there was such a token.
func (s *Store) Forget(ctx context.Context, token string) (bool, error) {
	res, err := s.DB.ExecContext(ctx, "DELETE FROM tokens WHERE token_hash = ?", TokenHash(token))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// TokenHash is what's stored of a token: a leaked database or backup doesn't let anyone become
// someone else. Rooms also tell browsers apart by it.
func TokenHash(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

// queryRower is a *sql.DB or a *sql.Tx.
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// byToken finds the user a token belongs to. An unknown token is sql.ErrNoRows.
func byToken(ctx context.Context, q queryRower, token string) (User, error) {
	return byHash(ctx, q, TokenHash(token))
}

func byHash(ctx context.Context, q queryRower, hash []byte) (User, error) {
	u := User{}
	err := q.QueryRowContext(ctx,
		"SELECT u.id, u.name FROM tokens t JOIN users u ON u.id = t.user_id WHERE t.token_hash = ?", hash,
	).Scan(&u.ID, &u.Name)
	return u, err
}

// ByTokenHash finds the user whose browser has this token hash (TokenHash): rooms know browsers by it.
// An unknown hash reports false, not an error.
func ByTokenHash(ctx context.Context, db *sql.DB, hash []byte) (User, bool, error) {
	u, err := byHash(ctx, db, hash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	return u, err == nil, err
}

// person finds the person with this name, or makes them. A found person keeps their own spelling.
// If the caller fails after this, all that's left is a person with no browser, as after a logout.
func person(ctx context.Context, q queryRower, name string) (User, error) {
	u := User{}
	// A no-op update rather than DO NOTHING: that returns no row when the name is taken.
	err := q.QueryRowContext(ctx, `INSERT INTO users (name, name_key) VALUES (?, ?)
		ON CONFLICT (name_key) DO UPDATE SET name = name RETURNING id, name`, name, NameKey(name),
	).Scan(&u.ID, &u.Name)
	return u, err
}
