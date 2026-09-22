# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

**PauseTogether**: *"Distance can't pause us."* A self-hosted watch-together web app. One host machine
serves its local video library. Guests reach it over Tailscale and watch in sync from PCs, tablets and
phones. Same show. Same second. Different places.

**Status: design only, no code yet.** This file is the spec. As code lands, update the layout and
commands below to match reality. The rules here are decided: flag problems, but don't quietly change them.

## Stack

- **Backend:** Go, standard library first. `net/http` routing with method/path patterns
  (`GET /api/rooms/{id}`), no web framework. `database/sql` with hand-written SQL, no ORM. `log/slog`.
  WebSockets via `github.com/coder/websocket`. SQLite via `modernc.org/sqlite` (pure Go, so
  `CGO_ENABLED=0` and simple Docker builds). `ffmpeg`/`ffprobe` run through `os/exec`.
- **Frontend:** SvelteKit with `adapter-static` in SPA mode (`fallback: 'index.html'`, `ssr = false` in
  the root layout), Svelte 5 runes, Tailwind v4 (config lives in CSS via `@theme`; there is no
  `tailwind.config.js`). The build is embedded into the Go binary. Go serves real files and falls back
  to `index.html` for app routes like `/rooms/{id}`.
- **Database:** one SQLite file in the data dir.
- **Docker** for both dev and prod. **Taskfile** wraps every command.

## Commands (planned)

```sh
task dev                                                  # dev stack, hot reload: http://localhost:5173
task test                                                 # all unit tests (Go + web), in Docker
task test:go -- -run TestParseEpisode ./internal/library  # one Go test
task test:web -- src/lib/sync                             # web tests under one path
task lint                                                 # gofmt, go vet, svelte-check
task build                                                # build the production image
task up | task down | task logs                           # start, stop, follow the production stack
```

Ask before `task up` or `task down`: they restart the live server, maybe mid-movie. Dev and prod use
different ports, compose project names and data dirs, so `task dev` is safe to run next to prod.

Config lives in `.env` (template: `.env.example`):
- `MEDIA_ROOT`: host folder holding all media, mounted read-only at `/media`.
- `DATA_DIR`: host folder for the database and cache, mounted at `/data`.
- `PUBLIC_BIND`: host IP for the guest port, default `0.0.0.0`.

## Layout (planned)

```
cmd/pausetogether/   main: config, wiring, both HTTP listeners
internal/api/        HTTP handlers, guest vs admin routes, SPA serving
internal/room/       room state, sync engine, presence, chat, WebSocket hub
internal/library/    folder scanning, Plex name parsing, file watching
internal/media/      ffprobe, prepare jobs, subtitles to WebVTT, cache
internal/store/      SQLite access; migrations/*.sql embedded and applied at startup
web/                 SvelteKit app; web/embed.go embeds its build (go:embed can't reach ../)
Dockerfile           web build → Go build → runtime image with ffmpeg
compose.yml          production
compose.dev.yml      development
Taskfile.yml
```

URL prefixes: `/api` JSON, `/api/admin` admin only, `/ws` WebSocket (one per open room page), `/media`
prepared videos and subtitles.

## Access and networking

- One process, two HTTP listeners:
  - **Guest port `:8080`**: the whole app except the admin API. Guests reach it over Tailscale.
  - **Admin port `:8081`**: the same app **plus** the admin API. Compose publishes it on `127.0.0.1`
    only, so only the host machine can reach it. The host uses `http://localhost:8081` for everything.
- Why ports and not IP checks: inside Docker, the host's own requests arrive from the Docker network
  gateway, not `127.0.0.1`, so the app can't tell host from guest by IP. Never add IP-based admin checks.
- Tailscale runs on the host OS, not in a container, and it is the only access control: anyone who
  reaches the guest port is trusted. Docker bypasses `ufw`, so the guest port is open on every network
  the host is on unless `PUBLIC_BIND` is set to the host's Tailscale IP.
- Plain HTTP over the tailnet (WireGuard already encrypts). Guest pages are **not** a secure context:
  no `crypto.randomUUID()`, `navigator.clipboard`, Wake Lock or other HTTPS-only browser APIs.
- Wrap state-changing routes in `http.CrossOriginProtection` (Go 1.25+, CSRF), above all on the admin
  port.
- In dev, Vite proxies `/api`, `/ws` and `/media` to Go, sending `/api/admin` to the admin listener.
- Bandwidth: every viewer streams the original file. The host's upload must cover bitrate × viewers,
  and a guest on a relayed (DERP) Tailscale link may buffer, which pauses the room.

## Users

- No accounts: one host (anyone on the admin port) plus anonymous guests.
- First visit: pick a display name. The server creates a user with a random token in an `HttpOnly`
  cookie (it rides along on the WebSocket upgrade too). The token keeps the name across visits.
- Everyone can: see all rooms, create rooms, control playback, switch a room's video, chat, archive and
  unarchive rooms.
- Host only (admin API): manage libraries (folder + type), rescan, delete rooms.
- Render all user text (names, chat) as text. Never `{@html}`.

## Library

- Local files only. A library is a folder under `/media` plus a type. The admin folder picker browses
  `/media` only. Never write into media folders.
- Types: **Movies**, **TV Shows**, **Other Videos**.
- Plex naming is required for Movies and TV Shows. Files that don't match are skipped and listed on the
  admin page with the reason:
  - Movies: `Title (Year)/Title (Year).ext`; a loose `Title (Year).ext` is fine too.
  - TV: `Show (Year)/Season 01/Show (Year) - s01e02 - Episode Title.ext`; year and episode title optional.
  - Other Videos: the file name is the title.
  - `{...}` tags such as `{edition-Director's Cut}` are ignored.
- Metadata comes from names only: no online lookups, no artwork. `ffprobe` supplies only technical
  facts (duration, codecs, audio and subtitle tracks).
- Sidecar subtitles sit next to the video with Plex names: `Title (Year).en.srt`,
  `Title (Year).en.forced.srt`.
- Watching: inotify watches one directory at a time, so add a watch per directory. Debounce events and
  wait until a new file stops growing before probing it. Also rescan on a timer and on the admin
  "Rescan" button, because events do get missed (network drives, inotify limits).
- Clients send IDs, never file paths. The admin folder picker is the one exception and must stay inside
  `/media`.

## Media pipeline

Browsers play only some codecs, and a remux streamed on the fly can't seek (no fixed length, no index).
So each room's video is **prepared once**, then served as a plain file.

- Prepare = an ffmpeg remux to MP4 in `/data/cache`:
  - video copied, never re-encoded (original quality). HEVC is tagged `hvc1` (`-tag:v hvc1`) or Apple
    devices refuse it.
  - only the room's chosen audio track (browsers can't switch audio tracks). Copied if it's AAC with at
    most two channels, otherwise converted to stereo AAC with a dialogue-friendly downmix (ffmpeg's
    default 5.1 → stereo downmix makes voices quiet).
  - `-movflags +faststart`, so the index sits at the front and playback starts right away.
- Every video goes through prepare, even ones a browser could play as-is. One code path, on purpose.
- Jobs run one at a time, start as soon as a room gets a video, and report progress to the room.
- Serve with `http.ServeContent`: Range requests give native `<video>` seeking. No HLS.
- Prepared copies are keyed by video + audio track, so rooms can share one. A copy is deleted once no
  unarchived room uses it; unarchiving prepares it again. The cache needs free space about the size of
  the videos in active rooms.
- Codecs: H.264 plays everywhere. HEVC, AV1 and VP9 play only on some devices. Old codecs (MPEG-2, VC-1,
  XviD) are marked unplayable at scan. Video is never transcoded, so when a device can't decode it
  (`canPlayType()` or the `<video>` error event), show "This device can't play HEVC", not a black screen.
- Subtitles: text tracks (embedded SRT/ASS/mov_text; sidecar `.srt`/`.vtt`/`.ass`) become WebVTT in
  the cache, in UTF-8. Sidecars often aren't UTF-8 (e.g. Windows-1254 Turkish), so detect and convert.
  ASS styling is lost. Image subtitles (PGS, VobSub) can't become text without OCR: list them as
  unavailable.
- Audio track, subtitle and subtitle offset are room state, shared by everyone. Subtitles render in our
  own overlay, not native captions, so the offset is a simple time shift and we control where the text
  sits (clear of chat toasts and controls).
- If the video is gone (source moved or deleted, and no prepared copy), the room shows "Video missing"
  with the library picker. The new pick resumes at the same position.

## Rooms and sync

- Anyone creates a room by picking a video, with audio track and subtitle defaults preselected.
- The homepage lists every room with its current video and who's watching. Rooms live until deleted.
  Archived rooms sit in their own section and can be unarchived.
- A room plays one video at a time. Anyone can switch it with the library picker, and TV episodes also
  get a "Next episode" button. A switch starts at 0:00; only the "Video missing" swap keeps the position.
- Progress is per room only: the position is saved on pause, seek and switch, and every few seconds
  while playing. No per-user progress.
- Presence: "watching now" (open socket) and "was here" (joined before, gone now), with a short
  reconnect grace so flaky phones don't flicker between the two.
- **The server is the source of truth.** Room state: video, playing, position + server timestamp,
  audio track, subtitle, subtitle offset. Clients send intents (play, pause, seek, switch, subtitle,
  offset) and status (ready, buffering, away). The server applies them and broadcasts the full state.
- Clients estimate their clock offset to the server (ping/pong, keep the lowest-RTT sample) and compute
  where playback should be. Small drift: nudge `playbackRate`. Big drift: seek.
- Only our own controls send intents. `<video>` events (a `pause` from a locked phone, `waiting`) are
  status, never commands. No native `controls` attribute.
- Buffering: a client stalled longer than a short grace period pauses the room ("Waiting for Alice"),
  and it resumes when everyone is ready. Anyone can press "Play anyway"; the stuck client is then
  skipped until it catches up. Hidden or backgrounded tabs count as away and never block.
- The room pauses when the last person leaves. On server start, every room loads paused.
- Browsers block autoplay with sound, so entering a room shows a "Tap to join" button first.
- Fullscreen the player wrapper, not the `<video>`, so chat and subtitles stay on top. Where
  `document.fullscreenEnabled` is false (iPhone Safari), fill the window with CSS instead.
- Keep sync logic pure (no IO, clock injected) on both sides, `internal/room` and `web/src/lib/sync`,
  so it can be unit-tested.

## Chat

- One chat per room, kept forever, deleted with the room (foreign-key cascade).
- Plain text. Emoji are ordinary Unicode typed on the device keyboard; each device draws its own.
- A message can reply to one earlier message and shows a short quote of it.
- Each message stores the video and the room position when it was sent (from the server's room clock)
  and shows that as a timestamp, plus the video's name if it isn't the one playing now.
- No typing indicator, no sounds, no system messages: play, pause, seek, switch, join and leave never
  appear in chat.
- UI: a side panel on wide screens, a bottom sheet on portrait phones. While it's closed, new messages
  pop up as small toasts over the video and fade out; tapping one opens a reply to it. Must work in
  fullscreen.

## Conventions

- Write unit tests for new logic: Go table-driven tests next to the code, Vitest for `web/src/lib`. The
  Plex name parser gets plenty of real-world file names. No end-to-end tests for now.
- SQLite: every connection sets `foreign_keys=ON` (off by default, and cascades silently don't run
  without it), WAL and `busy_timeout`, via `_pragma` in the DSN.
- Migrations: numbered `.sql` files in `internal/store/migrations`, tracked with `PRAGMA user_version`.
  Never edit an applied migration; add a new one.
- WebSocket messages are JSON `{"type": ..., ...}`, defined once per side (`internal/room/protocol.go`,
  `web/src/lib/protocol.ts`). Change both together.
- The frontend uses relative URLs only, so one build works on both ports and behind the dev proxy.
- LF line endings everywhere.
