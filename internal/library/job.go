package library

import (
	"context"
	"database/sql"
	"errors"
	"path"
	"path/filepath"
	"time"

	"github.com/erkanvatan/pause-together/internal/media"
)

// VideoRef is a video as a room shows it: missing ones too, so a room keeps its video's name.
type VideoRef struct {
	VideoSummary
	Missing bool `json:"missing"`
}

// Video returns a video, missing or not, or ErrNotFound.
func (l *Libraries) Video(ctx context.Context, id int64) (VideoRef, error) {
	var missing bool
	sum, err := scanSummary(l.DB.QueryRowContext(ctx,
		"SELECT v.missing,"+summaryColumns+" WHERE v.id = ?", id), &missing)
	if errors.Is(err, sql.ErrNoRows) {
		return VideoRef{}, ErrNotFound
	}
	return VideoRef{VideoSummary: sum, Missing: missing}, err
}

// PrepareJob returns the prepare job for a video and one of its audio streams (nil: no audio), or
// ErrNotFound. It works for a video that is gone too. Then missing is true, and the job can't run: the
// file, its library folder, or the audio track is gone.
func (l *Libraries) PrepareJob(ctx context.Context, videoID int64, audio *int) (j media.Job, missing bool, err error) {
	var libPath, videoPath string
	var durationMs int64
	err = l.DB.QueryRowContext(ctx, `
		SELECT l.path, v.path, v.video_codec, v.duration_ms, v.size, v.mtime, v.missing
		FROM videos v JOIN libraries l ON l.id = v.library_id WHERE v.id = ?`, videoID).
		Scan(&libPath, &videoPath, &j.VideoCodec, &durationMs, &j.Size, &j.Mtime, &missing)
	if errors.Is(err, sql.ErrNoRows) {
		return media.Job{}, false, ErrNotFound
	}
	if err != nil {
		return media.Job{}, false, err
	}
	j.VideoID = videoID
	j.Name = path.Join(libPath, videoPath)
	j.Duration = time.Duration(durationMs) * time.Millisecond

	if audio != nil {
		a := media.AudioTrack{Stream: *audio}
		err := l.DB.QueryRowContext(ctx, `
			SELECT codec, channels, layout, lang, title, is_default FROM audio_tracks WHERE video_id = ? AND stream = ?`,
			videoID, *audio).Scan(&a.Codec, &a.Channels, &a.Layout, &a.Lang, &a.Title, &a.Default)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			missing = true // probed again since the pick, and the track is gone
		case err != nil:
			return media.Job{}, false, err
		}
		j.Audio = &a
	}

	j.Subtitles, err = queryList(ctx, l.DB,
		"SELECT stream FROM subtitle_tracks WHERE video_id = ? AND unavailable IS NULL ORDER BY stream",
		func(rows *sql.Rows) (int, error) {
			var s int
			return s, rows.Scan(&s)
		}, videoID)
	if err != nil {
		return media.Job{}, false, err
	}

	// Resolved the same way a scan does. A library folder that is gone, or leads outside the media
	// folder, leaves the file out of reach until the next scan marks it missing.
	root, _, err := realPath(l.Root, libPath)
	if err != nil {
		return j, true, nil
	}
	j.Source = filepath.Join(root, filepath.FromSlash(videoPath))
	return j, missing, nil
}
