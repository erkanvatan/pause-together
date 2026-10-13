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

// ErrNameTaken means the name is another person's. Join makes a browser them.
var ErrNameTaken = errors.New("name taken")

// ErrUnknownPerson means no person has the id.
var ErrUnknownPerson = errors.New("unknown person")

// Store reads and writes users and their tokens. Names passed in must already be cleaned with CleanName.
type Store struct {
	DB *sql.DB
}

// Create gives a new browser a token, and makes it a new person with this name. A name someone has is
// ErrNameTaken, with that person in their spelling. Only the token's hash is stored.
func (s *Store) Create(ctx context.Context, name string) (User, string, error) {
	// One transaction, so the name's holder can't rename between finding the name taken and reading who.
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", err
	}
	defer func() { _ = tx.Rollback() }()
	key := NameKey(name)
	u := User{}
	err = tx.QueryRowContext(ctx, `INSERT INTO users (name, name_key) VALUES (?, ?)
		ON CONFLICT (name_key) DO NOTHING RETURNING id, name`, name, key).Scan(&u.ID, &u.Name)
	if errors.Is(err, sql.ErrNoRows) {
		if u, err = byName(ctx, tx, key); err != nil {
			return User{}, "", err
		}
		return u, "", ErrNameTaken
	}
	if err != nil {
		return User{}, "", err
	}
	token, err := addToken(ctx, tx, u.ID)
	if err != nil {
		return User{}, "", err
	}
	return u, token, tx.Commit()
}

// Join makes this browser the person with the id, and returns who it was and who it is now. A browser
// with no token, or an unknown one, gets a new token (from stays zero); a known one keeps its token,
// and the person it leaves keeps its name and its other browsers. It goes by id, not name: a person who
// renamed since the page asked is still the one it asked about. An unknown id is ErrUnknownPerson.
func (s *Store) Join(ctx context.Context, token string, id int64) (from, to User, newToken string, err error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, User{}, "", err
	}
	defer func() { _ = tx.Rollback() }()
	err = tx.QueryRowContext(ctx, "SELECT id, name FROM users WHERE id = ?", id).Scan(&to.ID, &to.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, User{}, "", ErrUnknownPerson
	}
	if err != nil {
		return User{}, User{}, "", err
	}
	if token != "" {
		from, err = byToken(ctx, tx, token)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return User{}, User{}, "", err
		}
	}
	if from.ID != 0 {
		_, err = tx.ExecContext(ctx, "UPDATE tokens SET user_id = ? WHERE token_hash = ?", to.ID, TokenHash(token))
	} else {
		newToken, err = addToken(ctx, tx, to.ID)
	}
	if err != nil {
		return User{}, User{}, "", err
	}
	return from, to, newToken, tx.Commit()
}

// addToken gives person id a new browser.
func addToken(ctx context.Context, tx *sql.Tx, id int64) (string, error) {
	token := rand.Text()
	_, err := tx.ExecContext(ctx, "INSERT INTO tokens (token_hash, user_id) VALUES (?, ?)", TokenHash(token), id)
	return token, err
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
// spelling. A name another person has is ErrNameTaken, with that person as to, and nothing changes:
// Join moves the browser to them. An unknown token is ErrUnknownToken.
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
	if n == 0 { // another person has the name
		if to, err = byName(ctx, tx, key); err != nil {
			return User{}, User{}, err
		}
		return from, to, ErrNameTaken
	}
	return from, User{ID: from.ID, Name: name}, tx.Commit()
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

// byName finds the person with this name key.
func byName(ctx context.Context, q queryRower, key string) (User, error) {
	u := User{}
	err := q.QueryRowContext(ctx, "SELECT id, name FROM users WHERE name_key = ?", key).Scan(&u.ID, &u.Name)
	return u, err
}
