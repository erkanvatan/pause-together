-- The host's cache settings. Always exactly one row.
CREATE TABLE cache_settings (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    -- a prepared copy nobody has used for this many days is deleted; media.MinUnusedDays to MaxUnusedDays
    unused_days INTEGER NOT NULL CHECK (unused_days BETWEEN 1 AND 365)
) STRICT;

INSERT INTO cache_settings (id, unused_days) VALUES (1, 7);
