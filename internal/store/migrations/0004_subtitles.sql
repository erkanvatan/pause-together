-- Why an embedded track can't become WebVTT (media.SubtitleUnavailable), e.g. 'image' for PGS.
-- NULL = prepare turns it into WebVTT.
ALTER TABLE subtitle_tracks ADD COLUMN unavailable TEXT;
-- Tracks probed before this column: unchanged files aren't probed again. The lists are
-- media.subtitleUnavailable's as of this migration.
UPDATE subtitle_tracks SET unavailable = CASE
    WHEN codec IN ('subrip', 'ass', 'ssa', 'mov_text', 'webvtt') THEN NULL
    WHEN codec IN ('hdmv_pgs_subtitle', 'dvd_subtitle', 'dvb_subtitle', 'xsub') THEN 'image'
    ELSE 'codec'
END;

-- Sidecar subtitle files, converted to WebVTT at scan. Unlike a video's, a row is deleted when its file
-- is gone, or can't be converted any more.
CREATE TABLE sidecar_subtitles (
    id        INTEGER PRIMARY KEY,
    video_id  INTEGER NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    name      TEXT NOT NULL, -- file name, in its video's folder
    lang      TEXT NOT NULL, -- from the file name, e.g. 'tr' or 'tur'
    forced    INTEGER NOT NULL,
    sdh       INTEGER NOT NULL,
    cache_key TEXT NOT NULL, -- media.SidecarKey: the converted copy in the cache
    UNIQUE (video_id, name)
) STRICT;
