-- A name is a person. users was one row per browser; now it is one row per name (name_key, made by
-- the name_key function: user.NameKey), and tokens maps each browser's token to its person. Users
-- whose names match merge into the oldest: its spelling stays, and every token and message moves to it.

-- Each user, its name's key, and the user it merges into: the oldest with that key.
CREATE TEMP TABLE merge AS
    SELECT id, key, min(id) OVER (PARTITION BY key) AS keep
    FROM (SELECT id, name_key(name) AS key FROM users);

-- token_hash is SHA-256 of the cookie's token: a leaked database or backup doesn't let anyone become
-- someone else.
CREATE TABLE tokens (
    token_hash BLOB PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id)
) STRICT, WITHOUT ROWID;

INSERT INTO tokens (token_hash, user_id)
    SELECT u.token_hash, m.keep FROM users u JOIN merge m ON m.id = u.id;

UPDATE messages SET user_id = (SELECT keep FROM merge WHERE id = messages.user_id)
    WHERE user_id IN (SELECT id FROM merge WHERE id <> keep);

-- A user with no token left stays: their messages still show their name.
CREATE TABLE users_new (
    id       INTEGER PRIMARY KEY,
    name     TEXT NOT NULL,
    name_key TEXT NOT NULL UNIQUE
) STRICT;

INSERT INTO users_new (id, name, name_key)
    SELECT u.id, u.name, m.key FROM users u JOIN merge m ON m.id = u.id WHERE m.id = m.keep;

DROP TABLE users;
ALTER TABLE users_new RENAME TO users;
DROP TABLE merge;
