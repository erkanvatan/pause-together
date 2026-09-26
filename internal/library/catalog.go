package library

import (
	"context"
	"database/sql"
	"errors"
)

// VideoSummary is a video as the picker lists it. Name fields are as in Video: 0 and "" mean none,
// except Season 0, which is specials.
type VideoSummary struct {
	ID           int64  `json:"id"`
	Type         Type   `json:"type"` // its library's
	Title        string `json:"title"`
	Year         int    `json:"year"`
	Edition      string `json:"edition"`
	Version      string `json:"version"`
	Season       int    `json:"season"`
	Episode      int    `json:"episode"`
	EpisodeEnd   int    `json:"episodeEnd"`
	EpisodeTitle string `json:"episodeTitle"`
	Group        string `json:"group"`
	DurationMs   int64  `json:"durationMs"`
	CodecString  string `json:"codecString"` // for canPlayType()
	Unplayable   string `json:"unplayable"`  // a media.Unplayable; "" = plays
	AppleOnly    bool   `json:"appleOnly"`   // Dolby Vision profile 5: other screens show it purple and green
}

// VideoDetail is a video with the tracks the picker offers. Every lang is NormalizeLang's.
type VideoDetail struct {
	VideoSummary
	Audio     []AudioInfo    `json:"audio"`
	Subtitles []SubtitleInfo `json:"subtitles"`
	Sidecars  []SidecarInfo  `json:"sidecars"`
}

type AudioInfo struct {
	Stream   int    `json:"stream"`
	Codec    string `json:"codec"`
	Channels int    `json:"channels"`
	Lang     string `json:"lang"`
	Title    string `json:"title"`
	Default  bool   `json:"default"`
}

// SubtitleInfo is an embedded subtitle track.
type SubtitleInfo struct {
	Stream      int    `json:"stream"`
	Lang        string `json:"lang"`
	Title       string `json:"title"`
	Default     bool   `json:"default"`
	Forced      bool   `json:"forced"`
	SDH         bool   `json:"sdh"`
	Unavailable string `json:"unavailable"` // a media.SubtitleUnavailable; "" = can be shown
}

// SidecarInfo is a sidecar subtitle file.
type SidecarInfo struct {
	ID     int64  `json:"id"`
	Lang   string `json:"lang"`
	Forced bool   `json:"forced"`
	SDH    bool   `json:"sdh"`
}

// summaryColumns match scanSummary. A removed library's videos are all missing, so "NOT missing" is
// enough to leave them out.
const summaryColumns = `
	v.id, l.type, v.title, v.year, v.edition, v.version, v.season, v.episode, v.episode_end, v.episode_title,
	v.group_name, v.duration_ms, v.codec_string, COALESCE(v.unplayable, ''), v.apple_only
	FROM videos v JOIN libraries l ON l.id = v.library_id`

func scanSummary(row interface{ Scan(...any) error }) (VideoSummary, error) {
	var v VideoSummary
	err := row.Scan(&v.ID, &v.Type, &v.Title, &v.Year, &v.Edition, &v.Version, &v.Season, &v.Episode,
		&v.EpisodeEnd, &v.EpisodeTitle, &v.Group, &v.DurationMs, &v.CodecString, &v.Unplayable, &v.AppleOnly)
	return v, err
}

// Videos lists every video that isn't missing, by id: the order they were added.
func (l *Libraries) Videos(ctx context.Context) ([]VideoSummary, error) {
	return queryList(ctx, l.DB, "SELECT"+summaryColumns+" WHERE NOT v.missing ORDER BY v.id",
		func(rows *sql.Rows) (VideoSummary, error) { return scanSummary(rows) })
}

// VideoDetail returns a video that isn't missing, with its tracks, or ErrNotFound.
func (l *Libraries) VideoDetail(ctx context.Context, id int64) (VideoDetail, error) {
	sum, err := scanSummary(l.DB.QueryRowContext(ctx, "SELECT"+summaryColumns+" WHERE v.id = ? AND NOT v.missing", id))
	if errors.Is(err, sql.ErrNoRows) {
		return VideoDetail{}, ErrNotFound
	}
	if err != nil {
		return VideoDetail{}, err
	}
	d := VideoDetail{VideoSummary: sum}

	d.Audio, err = queryList(ctx, l.DB, `
		SELECT stream, codec, channels, lang, title, is_default FROM audio_tracks WHERE video_id = ? ORDER BY stream`,
		func(rows *sql.Rows) (AudioInfo, error) {
			var a AudioInfo
			err := rows.Scan(&a.Stream, &a.Codec, &a.Channels, &a.Lang, &a.Title, &a.Default)
			a.Lang, _ = NormalizeLang(a.Lang)
			return a, err
		}, id)
	if err != nil {
		return VideoDetail{}, err
	}
	d.Subtitles, err = queryList(ctx, l.DB, `
		SELECT stream, lang, title, is_default, forced, sdh, COALESCE(unavailable, '')
		FROM subtitle_tracks WHERE video_id = ? ORDER BY stream`,
		func(rows *sql.Rows) (SubtitleInfo, error) {
			var s SubtitleInfo
			err := rows.Scan(&s.Stream, &s.Lang, &s.Title, &s.Default, &s.Forced, &s.SDH, &s.Unavailable)
			s.Lang, _ = NormalizeLang(s.Lang)
			return s, err
		}, id)
	if err != nil {
		return VideoDetail{}, err
	}
	d.Sidecars, err = queryList(ctx, l.DB, `
		SELECT id, lang, forced, sdh FROM sidecar_subtitles WHERE video_id = ? ORDER BY name`,
		func(rows *sql.Rows) (SidecarInfo, error) {
			var s SidecarInfo
			err := rows.Scan(&s.ID, &s.Lang, &s.Forced, &s.SDH)
			s.Lang, _ = NormalizeLang(s.Lang)
			return s, err
		}, id)
	if err != nil {
		return VideoDetail{}, err
	}
	return d, nil
}

// queryList runs a query and scans each row with scan. No rows gives an empty list, not nil, so it
// goes out as [] in JSON.
func queryList[T any](ctx context.Context, db *sql.DB, query string, scan func(*sql.Rows) (T, error), args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}
