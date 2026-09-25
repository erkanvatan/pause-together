-- One row per browser that picked a name. token_hash is SHA-256 of the cookie's token: a leaked
-- database or backup doesn't let anyone become someone else.
CREATE TABLE users (
    id         INTEGER PRIMARY KEY,
    token_hash BLOB NOT NULL UNIQUE,
    name       TEXT NOT NULL
) STRICT;
