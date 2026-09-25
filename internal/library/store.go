package library

import (
	"context"
	"database/sql"
	"errors"

	"github.com/erkanvatan/pause-together/internal/media"
	"github.com/erkanvatan/pause-together/internal/store"
)

// knownVideo is what a scan needs to know about a row it already has.
type knownVideo struct {
	id      int64
	size    int64
	mtime   int64
	failed  bool // the last probe failed
	missing bool
	video   Video // the name fields as stored
}

// parsedVideo is an unchanged file whose row needs its name fields or missing flag written again.
type parsedVideo struct {
	id    int64
	video Video
}

type skippedFile struct {
	path   string
	reason Reason
}

// activeLibraries returns every library that isn't removed.
func activeLibraries(ctx context.Context, db *sql.DB) ([]Library, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, path, type FROM libraries WHERE NOT removed ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var libs []Library
	for rows.Next() {
		var l Library
		if err := rows.Scan(&l.ID, &l.Path, &l.Type); err != nil {
			return nil, err
		}
		libs = append(libs, l)
	}
	return libs, rows.Err()
}

// getLibrary returns a library that isn't removed, or ErrNotFound.
func getLibrary(ctx context.Context, db *sql.DB, id int64) (Library, error) {
	var l Library
	err := db.QueryRowContext(ctx, "SELECT id, path, type FROM libraries WHERE id = ? AND NOT removed", id).
		Scan(&l.ID, &l.Path, &l.Type)
	if errors.Is(err, sql.ErrNoRows) {
		return Library{}, ErrNotFound
	}
	return l, err
}

// checkActive returns ErrRemoved if the library was removed. Scans call it first in each of their
// transactions: a removal can't run between the check and the writes, so a scan that was running
// when its library was removed can never bring its videos back.
func checkActive(ctx context.Context, tx *sql.Tx, libraryID int64) error {
	var removed bool
	err := tx.QueryRowContext(ctx, "SELECT removed FROM libraries WHERE id = ?", libraryID).Scan(&removed)
	if errors.Is(err, sql.ErrNoRows) || err == nil && removed {
		return ErrRemoved
	}
	return err
}

// knownVideos returns the library's video rows by path.
func (s *Scanner) knownVideos(ctx context.Context, libraryID int64) (map[string]knownVideo, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, path, size, mtime, probe_error IS NOT NULL, missing, title, year, edition, version, season,
			episode, episode_end, episode_title, group_name
		FROM videos WHERE library_id = ?`, libraryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	known := make(map[string]knownVideo)
	for rows.Next() {
		var p string
		var k knownVideo
		v := &k.video
		if err := rows.Scan(&k.id, &p, &k.size, &k.mtime, &k.failed, &k.missing, &v.Title, &v.Year, &v.Edition,
			&v.Version, &v.Season, &v.Episode, &v.EpisodeEnd, &v.EpisodeTitle, &v.Group); err != nil {
			return nil, err
		}
		known[p] = k
	}
	return known, rows.Err()
}

// saveProbed writes a probed video and replaces its tracks. perr is the probe's error, if it failed.
func (s *Scanner) saveProbed(ctx context.Context, libraryID int64, rel string, v Video, st fileStat,
	info media.Info, perr error) error {
	var probeErr, unplayable sql.NullString
	if perr != nil {
		probeErr = sql.NullString{String: perr.Error(), Valid: true}
		info = media.Info{Unplayable: media.UnplayableProbeFailed} // no stale facts from an older probe
	}
	if info.Unplayable != "" {
		unplayable = sql.NullString{String: string(info.Unplayable), Valid: true}
	}

	return store.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		if err := checkActive(ctx, tx, libraryID); err != nil {
			return err
		}
		var id int64
		err := tx.QueryRowContext(ctx, `
			INSERT INTO videos (library_id, path, title, year, edition, version, season, episode, episode_end,
				episode_title, group_name, size, mtime, missing, probe_error, duration_ms, video_codec,
				codec_string, unplayable, apple_only)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (library_id, path) DO UPDATE SET
				title = excluded.title, year = excluded.year, edition = excluded.edition,
				version = excluded.version, season = excluded.season, episode = excluded.episode,
				episode_end = excluded.episode_end, episode_title = excluded.episode_title,
				group_name = excluded.group_name, size = excluded.size, mtime = excluded.mtime, missing = 0,
				probe_error = excluded.probe_error, duration_ms = excluded.duration_ms,
				video_codec = excluded.video_codec, codec_string = excluded.codec_string,
				unplayable = excluded.unplayable, apple_only = excluded.apple_only
			RETURNING id`,
			libraryID, rel, v.Title, v.Year, v.Edition, v.Version, v.Season, v.Episode, v.EpisodeEnd,
			v.EpisodeTitle, v.Group, st.size, st.mtime, probeErr, info.Duration.Milliseconds(), info.VideoCodec,
			info.CodecString, unplayable, info.AppleOnly,
		).Scan(&id)
		if err != nil {
			return err
		}

		for _, q := range []string{"DELETE FROM audio_tracks WHERE video_id = ?", "DELETE FROM subtitle_tracks WHERE video_id = ?"} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return err
			}
		}
		for _, a := range info.Audio {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO audio_tracks (video_id, stream, codec, channels, layout, lang, title, is_default)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				id, a.Stream, a.Codec, a.Channels, a.Layout, a.Lang, a.Title, a.Default); err != nil {
				return err
			}
		}
		for _, sub := range info.Subtitles {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO subtitle_tracks (video_id, stream, codec, lang, title, is_default, forced, sdh)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				id, sub.Stream, sub.Codec, sub.Lang, sub.Title, sub.Default, sub.Forced, sub.SDH); err != nil {
				return err
			}
		}
		return nil
	})
}

// finishScan writes the rest of a scan in one transaction: unchanged files whose name fields or
// missing flag need writing, gone videos marked missing, and the skipped list replaced.
func (s *Scanner) finishScan(ctx context.Context, libraryID int64, refresh []parsedVideo, gone []int64,
	skipped []skippedFile) error {
	return store.InTx(ctx, s.DB, func(tx *sql.Tx) error {
		if err := checkActive(ctx, tx, libraryID); err != nil {
			return err
		}
		for _, u := range refresh {
			v := u.video
			if _, err := tx.ExecContext(ctx, `
				UPDATE videos SET title = ?, year = ?, edition = ?, version = ?, season = ?, episode = ?,
					episode_end = ?, episode_title = ?, group_name = ?, missing = 0
				WHERE id = ?`,
				v.Title, v.Year, v.Edition, v.Version, v.Season, v.Episode, v.EpisodeEnd, v.EpisodeTitle, v.Group,
				u.id); err != nil {
				return err
			}
		}
		for _, id := range gone {
			if _, err := tx.ExecContext(ctx, "UPDATE videos SET missing = 1 WHERE id = ?", id); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM skipped_files WHERE library_id = ?", libraryID); err != nil {
			return err
		}
		for _, sk := range skipped {
			if _, err := tx.ExecContext(ctx, "INSERT INTO skipped_files (library_id, path, reason) VALUES (?, ?, ?)",
				libraryID, sk.path, string(sk.reason)); err != nil {
				return err
			}
		}
		return nil
	})
}
