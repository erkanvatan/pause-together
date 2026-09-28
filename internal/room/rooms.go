// Package room holds rooms: the video each one plays, and the prepare jobs they need.
package room

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/erkanvatan/pause-together/internal/library"
	"github.com/erkanvatan/pause-together/internal/media"
	"github.com/erkanvatan/pause-together/internal/user"
)

var (
	ErrNotFound = errors.New("no such room")
	ErrArchived = errors.New("room is archived")
	ErrBadPick  = errors.New("the video, audio track or subtitle can't be picked")
	ErrBadName  = user.ErrBadName
)

// Subtitle is an embedded subtitle stream or a sidecar file: exactly one is set.
type Subtitle struct {
	Stream  *int   `json:"stream,omitempty"`
	Sidecar *int64 `json:"sidecar,omitempty"`
}

// Pick is a video with its audio track (nil: the video has none) and subtitle (nil: off).
type Pick struct {
	VideoID  int64     `json:"videoId"`
	Audio    *int      `json:"audio"`
	Subtitle *Subtitle `json:"subtitle"`
}

// Room is a room as the pages show it.
type Room struct {
	ID         int64            `json:"id"`
	Name       string           `json:"name"` // "" = none: show the video's name
	Video      library.VideoRef `json:"video"`
	Audio      *int             `json:"audio"`
	Subtitle   *Subtitle        `json:"subtitle"`
	PositionMs int64            `json:"positionMs"`
	// SubtitleOffsetMs shifts the subtitle: positive shows it later.
	SubtitleOffsetMs int64 `json:"subtitleOffsetMs"`
	Archived         bool  `json:"archived"`
}

// Rooms keeps rooms in the database, and asks Jobs for the prepared copies they need.
type Rooms struct {
	DB      *sql.DB
	Library *library.Libraries
	Jobs    *media.Jobs

	// mu keeps each room change and its Add or Cancel together. Otherwise a room switching away could
	// cancel a job another room has just asked for.
	mu sync.Mutex
}

const roomColumns = "id, name, video_id, audio_stream, subtitle_stream, subtitle_sidecar, position_ms, subtitle_offset_ms, archived"

// row is a room as stored: Room without its video's details.
type row struct {
	Room
	videoID int64
}

func scanRow(r interface{ Scan(...any) error }) (row, error) {
	var rw row
	var subStream *int
	var subSidecar *int64
	err := r.Scan(&rw.ID, &rw.Name, &rw.videoID, &rw.Audio, &subStream, &subSidecar, &rw.PositionMs, &rw.SubtitleOffsetMs, &rw.Archived)
	if subStream != nil || subSidecar != nil {
		rw.Subtitle = &Subtitle{Stream: subStream, Sidecar: subSidecar}
	}
	return rw, err
}

// withVideo fills in the room's video.
func (r *Rooms) withVideo(ctx context.Context, rw row) (Room, error) {
	v, err := r.Library.Video(ctx, rw.videoID)
	rw.Video = v
	return rw.Room, err
}

// List returns every room, newest first.
func (r *Rooms) List(ctx context.Context) ([]Room, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT "+roomColumns+" FROM rooms ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	var stored []row
	for rows.Next() {
		rw, err := scanRow(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		stored = append(stored, rw)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	rooms := make([]Room, len(stored))
	for i, rw := range stored {
		if rooms[i], err = r.withVideo(ctx, rw); err != nil {
			return nil, err
		}
	}
	return rooms, nil
}

func (r *Rooms) get(ctx context.Context, id int64) (row, error) {
	rw, err := scanRow(r.DB.QueryRowContext(ctx, "SELECT "+roomColumns+" FROM rooms WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return row{}, ErrNotFound
	}
	return rw, err
}

// Open returns a room someone opens, and queues its prepare unless the room is archived, or its video
// is gone or no longer playable (a rescan found a new file).
func (r *Rooms) Open(ctx context.Context, id int64) (Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rm, err := r.opened(ctx, id)
	if err != nil || rm.Archived || rm.Video.Missing || rm.Video.Unplayable != "" {
		return rm, err
	}
	return rm, r.need(ctx, rm.Video.ID, rm.Audio)
}

// Create makes a room with no name. Its prepare is queued when the room is opened, which its creator
// does next.
func (r *Rooms) Create(ctx context.Context, p Pick) (Room, error) {
	if err := r.check(ctx, p); err != nil {
		return Room{}, err
	}
	stream, sidecar := p.subtitle()
	r.mu.Lock()
	defer r.mu.Unlock()
	var id int64
	if err := r.DB.QueryRowContext(ctx, `
		INSERT INTO rooms (name, video_id, audio_stream, subtitle_stream, subtitle_sidecar)
		VALUES ('', ?, ?, ?, ?) RETURNING id`, p.VideoID, p.Audio, stream, sidecar).Scan(&id); err != nil {
		return Room{}, err
	}
	return r.opened(ctx, id)
}

// Switch changes a room's video, back to 0:00. The new video's prepare is queued, and the old one's
// cancelled unless another room still needs it. An archived room can't switch.
func (r *Rooms) Switch(ctx context.Context, id int64, p Pick) (Room, error) {
	if err := r.check(ctx, p); err != nil {
		return Room{}, err
	}
	stream, sidecar := p.subtitle()
	r.mu.Lock()
	defer r.mu.Unlock()
	old, err := r.get(ctx, id)
	if err != nil {
		return Room{}, err
	}
	if old.Archived {
		return Room{}, ErrArchived
	}
	if _, err := r.DB.ExecContext(ctx, `
		UPDATE rooms SET video_id = ?, audio_stream = ?, subtitle_stream = ?, subtitle_sidecar = ?, position_ms = 0
		WHERE id = ?`, p.VideoID, p.Audio, stream, sidecar, id); err != nil {
		return Room{}, err
	}
	if err := r.need(ctx, p.VideoID, p.Audio); err != nil {
		return Room{}, err
	}
	if err := r.release(ctx, old.videoID, old.Audio); err != nil {
		return Room{}, err
	}
	return r.opened(ctx, id)
}

// Rename sets a room's name. Blank clears it; otherwise it follows the rules for user names.
func (r *Rooms) Rename(ctx context.Context, id int64, name string) (Room, error) {
	name = strings.TrimSpace(name)
	if name != "" {
		var err error
		if name, err = user.CleanName(name); err != nil {
			return Room{}, err
		}
	}
	res, err := r.DB.ExecContext(ctx, "UPDATE rooms SET name = ? WHERE id = ?", name, id)
	if err := notFound(res, err); err != nil {
		return Room{}, err
	}
	return r.opened(ctx, id)
}

// SetArchived archives or unarchives a room. Archiving cancels its prepare unless another room still
// needs it.
func (r *Rooms) SetArchived(ctx context.Context, id int64, archived bool) (Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rw, err := r.get(ctx, id)
	if err != nil {
		return Room{}, err
	}
	if _, err := r.DB.ExecContext(ctx, "UPDATE rooms SET archived = ? WHERE id = ?", archived, id); err != nil {
		return Room{}, err
	}
	if archived {
		if err := r.release(ctx, rw.videoID, rw.Audio); err != nil {
			return Room{}, err
		}
	}
	return r.opened(ctx, id)
}

// Delete deletes a room, and cancels its prepare unless another room still needs it. Its video row
// stays.
func (r *Rooms) Delete(ctx context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rw, err := r.get(ctx, id)
	if err != nil {
		return err
	}
	if _, err := r.DB.ExecContext(ctx, "DELETE FROM rooms WHERE id = ?", id); err != nil {
		return err
	}
	return r.release(ctx, rw.videoID, rw.Audio)
}

// SaveState stores the room state the loop keeps: position (st.PositionMs, as is), subtitle and offset.
// It writes only while the room still plays st's video, so a save that lands after a switch can't
// carry the old video's position over. A sidecar the scan has deleted since is saved as off.
func (r *Rooms) SaveState(ctx context.Context, id int64, st State) error {
	stream, sidecar := Pick{Subtitle: st.Subtitle}.subtitle()
	_, err := r.DB.ExecContext(ctx, `
		UPDATE rooms SET position_ms = ?, subtitle_stream = ?,
			subtitle_sidecar = (SELECT id FROM sidecar_subtitles WHERE id = ?), subtitle_offset_ms = ?
		WHERE id = ? AND video_id = ?`, st.PositionMs, stream, sidecar, st.SubtitleOffsetMs, id, st.VideoID)
	return err
}

// Visit records that a user joined a room, for its "was here" list.
func (r *Rooms) Visit(ctx context.Context, roomID, userID int64) error {
	_, err := r.DB.ExecContext(ctx, "INSERT OR IGNORE INTO room_visitors (room_id, user_id) VALUES (?, ?)", roomID, userID)
	return err
}

// Visitors returns everyone who ever joined a room, with their current names.
func (r *Rooms) Visitors(ctx context.Context, roomID int64) ([]Who, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT u.id, u.name FROM room_visitors v JOIN users u ON u.id = v.user_id
		WHERE v.room_id = ? ORDER BY u.name, u.id`, roomID)
	if err != nil {
		return nil, err
	}
	var who []Who
	for rows.Next() {
		var w Who
		if err := rows.Scan(&w.UserID, &w.Name); err != nil {
			_ = rows.Close()
			return nil, err
		}
		who = append(who, w)
	}
	return who, errors.Join(rows.Err(), rows.Close())
}

// opened reads a room back after a change.
func (r *Rooms) opened(ctx context.Context, id int64) (Room, error) {
	rw, err := r.get(ctx, id)
	if err != nil {
		return Room{}, err
	}
	return r.withVideo(ctx, rw)
}

// need queues the prepare of a video and audio track, unless the video is gone.
func (r *Rooms) need(ctx context.Context, videoID int64, audio *int) error {
	j, missing, err := r.Library.PrepareJob(ctx, videoID, audio)
	if err != nil || missing {
		return err
	}
	r.Jobs.Add(j)
	return nil
}

// release cancels the prepare of a video and audio track if no room that isn't archived still has
// them. It runs after the room's own change, so the room no longer counts.
func (r *Rooms) release(ctx context.Context, videoID int64, audio *int) error {
	var needed bool
	if err := r.DB.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM rooms WHERE video_id = ? AND audio_stream IS ? AND NOT archived)`,
		videoID, audio).Scan(&needed); err != nil || needed {
		return err
	}
	r.Jobs.Cancel(videoID, audio)
	return nil
}

// check returns ErrBadPick unless the video is there and playable, the audio track is one of its own
// (or nil when it has none), and the subtitle is off, one of its text tracks, or one of its sidecars.
func (r *Rooms) check(ctx context.Context, p Pick) error {
	d, err := r.Library.VideoDetail(ctx, p.VideoID)
	if errors.Is(err, library.ErrNotFound) {
		return ErrBadPick
	}
	if err != nil {
		return err
	}
	if d.Unplayable != "" {
		return ErrBadPick
	}
	if p.Audio == nil {
		if len(d.Audio) > 0 {
			return ErrBadPick
		}
	} else if !slices.ContainsFunc(d.Audio, func(a library.AudioInfo) bool { return a.Stream == *p.Audio }) {
		return ErrBadPick
	}
	ok := true
	switch s := p.Subtitle; {
	case s == nil:
	case s.Stream != nil && s.Sidecar == nil:
		ok = slices.ContainsFunc(d.Subtitles, func(t library.SubtitleInfo) bool {
			return t.Stream == *s.Stream && t.Unavailable == ""
		})
	case s.Sidecar != nil && s.Stream == nil:
		ok = slices.ContainsFunc(d.Sidecars, func(t library.SidecarInfo) bool { return t.ID == *s.Sidecar })
	default:
		ok = false
	}
	if !ok {
		return ErrBadPick
	}
	return nil
}

// subtitle splits the subtitle into its two columns.
func (p Pick) subtitle() (stream *int, sidecar *int64) {
	if p.Subtitle == nil {
		return nil, nil
	}
	return p.Subtitle.Stream, p.Subtitle.Sidecar
}

// notFound turns an UPDATE that changed no row into ErrNotFound.
func notFound(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}
