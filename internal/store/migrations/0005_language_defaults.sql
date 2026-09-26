-- The host's language defaults for new picks. Always exactly one row. Codes are
-- library.NormalizeLang's.
CREATE TABLE language_defaults (
    id        INTEGER PRIMARY KEY CHECK (id = 1),
    audio     TEXT NOT NULL, -- '' = original: the file's default track
    subtitles TEXT NOT NULL  -- in order of preference, comma-separated; '' = none
) STRICT;

INSERT INTO language_defaults (id, audio, subtitles) VALUES (1, '', '');
