-- A room plays one video at a time, with the audio track and subtitle picked for it. Rooms live until
-- the host deletes them.
CREATE TABLE rooms (
    -- AUTOINCREMENT: a deleted room's id is never reused, so an old link or open tab can't lead to
    -- another room.
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    name             TEXT NOT NULL, -- '' = none: the page shows the video's name
    -- Video rows are never deleted, so no cascade.
    video_id         INTEGER NOT NULL REFERENCES videos (id),
    audio_stream     INTEGER, -- ffprobe's stream index; NULL = the video has no audio
    -- The subtitle: an embedded stream, a sidecar, or neither (off). The scan deletes a sidecar's row
    -- when its file goes, and the room's subtitle turns off.
    subtitle_stream  INTEGER,
    subtitle_sidecar INTEGER REFERENCES sidecar_subtitles (id) ON DELETE SET NULL,
    position_ms      INTEGER NOT NULL DEFAULT 0,
    archived         INTEGER NOT NULL DEFAULT 0,
    CHECK (subtitle_stream IS NULL OR subtitle_sidecar IS NULL)
) STRICT;
