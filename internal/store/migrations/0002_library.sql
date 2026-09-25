-- A folder of videos plus its type. path is relative to the media folder (/media in the container).
CREATE TABLE libraries (
    id   INTEGER PRIMARY KEY,
    path TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL CHECK (type IN ('movies', 'tv', 'other'))
) STRICT;

-- One row per video file ever seen. A video is its library + its path in it, so a rename makes a new
-- row. Rows are never deleted: rooms and chat point at them. A gone file is marked missing.
-- ids only grow, so id order is "recently added".
CREATE TABLE videos (
    id            INTEGER PRIMARY KEY,
    library_id    INTEGER NOT NULL REFERENCES libraries (id),
    path          TEXT NOT NULL, -- relative to the library folder, "/" separators

    -- From the file name (library.Video); 0 and '' mean none, except season 0, which is specials.
    title         TEXT NOT NULL,
    year          INTEGER NOT NULL,
    edition       TEXT NOT NULL,
    version       TEXT NOT NULL,
    season        INTEGER NOT NULL,
    episode       INTEGER NOT NULL,
    episode_end   INTEGER NOT NULL,
    episode_title TEXT NOT NULL,
    group_name    TEXT NOT NULL,

    -- size and mtime: the file at its last probe. A change means probe again.
    size          INTEGER NOT NULL,
    mtime         INTEGER NOT NULL, -- Unix nanoseconds
    missing       INTEGER NOT NULL DEFAULT 0,

    -- From ffprobe. When the probe failed, probe_error holds ffprobe's message, unplayable is
    -- 'probe-failed', and the next scan retries it.
    probe_error   TEXT,
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    video_codec   TEXT NOT NULL DEFAULT '', -- ffprobe's name, e.g. 'vp8'
    codec_string  TEXT NOT NULL DEFAULT '', -- for canPlayType(), e.g. 'avc1.640028'
    unplayable    TEXT,                     -- reason code (media.Unplayable); NULL = plays
    apple_only    INTEGER NOT NULL DEFAULT 0, -- Dolby Vision profile 5

    UNIQUE (library_id, path)
) STRICT;

-- stream is ffprobe's stream index, as in ffmpeg's "-map 0:N". Replaced on every probe.
CREATE TABLE audio_tracks (
    video_id   INTEGER NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    stream     INTEGER NOT NULL,
    codec      TEXT NOT NULL,
    channels   INTEGER NOT NULL,
    layout     TEXT NOT NULL,
    lang       TEXT NOT NULL,
    title      TEXT NOT NULL,
    is_default INTEGER NOT NULL,
    PRIMARY KEY (video_id, stream)
) STRICT;

-- Embedded subtitle streams, text and image alike.
CREATE TABLE subtitle_tracks (
    video_id   INTEGER NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    stream     INTEGER NOT NULL,
    codec      TEXT NOT NULL,
    lang       TEXT NOT NULL,
    title      TEXT NOT NULL,
    is_default INTEGER NOT NULL,
    forced     INTEGER NOT NULL,
    sdh        INTEGER NOT NULL,
    PRIMARY KEY (video_id, stream)
) STRICT;

-- Files a scan couldn't use, with a reason code (library.Reason). Rewritten on every full scan of the
-- library, so unlike videos these rows come and go.
CREATE TABLE skipped_files (
    library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    reason     TEXT NOT NULL,
    PRIMARY KEY (library_id, path)
) STRICT;
