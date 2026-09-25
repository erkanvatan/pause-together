# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

**PauseTogether**: *"Distance can't pause us."* A self-hosted watch-together web app. One host machine
serves its local video library. Guests reach it over Tailscale and watch in sync from PCs, tablets and
phones. Same show. Same second. Different places.

**Status: being built in slices; `docs/plan.md` ticks off the done ones.** This file is the spec. As code
lands, update the layout and commands below to match reality. The rules here are decided: flag problems,
but don't quietly change them.

## Stack

- **Backend:** Go, standard library first. `net/http` routing with method/path patterns
  (`GET /api/rooms/{id}`), no web framework. `database/sql` with hand-written SQL, no ORM. `log/slog`.
  WebSockets via `github.com/coder/websocket`. SQLite via `modernc.org/sqlite` (pure Go, so
  `CGO_ENABLED=0` and simple Docker builds). File watching via `github.com/fsnotify/fsnotify`.
  `ffmpeg`/`ffprobe` run through `os/exec`.
- **Frontend:** SvelteKit with `adapter-static` in SPA mode (`fallback: 'index.html'`, `ssr = false` in
  the root layout), Svelte 5 runes, Tailwind v4 (config lives in CSS via `@theme`; there is no
  `tailwind.config.js`). Go serves real files and falls back to `index.html` for app routes like
  `/rooms/{id}`.
- **Embedding:** the web build is embedded into the Go binary with `//go:embed all:build`. Plain
  `build` would skip `build/_app`, where all the JS lives: `go:embed` leaves out names starting with `_`
  or `.`. `web/build/.gitkeep` is committed so Go compiles before any web build exists. SvelteKit empties
  `build/` on every build, so the `build` script in `web/package.json` recreates `.gitkeep` afterwards.
- **Database:** one SQLite file in the data dir.
- **Docker** for both dev and prod. **Taskfile** wraps every command.
- **Versions:** `go.mod` says `go 1.25.0`, the minimum (`http.CrossOriginProtection` needs 1.25, `os.Root`
  needs 1.24). The Dockerfile builds with the current stable Go, `golang:1.27`: Go only patches the last
  two releases. The web build uses Node LTS, `node:24-slim`. The runtime image is `debian:trixie-slim`
  with its `ffmpeg` package (7.1), patched by Debian. Not `jrottenberg/ffmpeg`: one maintainer, and a
  newer ffmpeg buys nothing, since video is never re-encoded.

## Commands

```sh
task dev                                                      # dev stack, hot reload: http://localhost:5173
task test                                                     # all unit tests (Go + web), in Docker
task test:go -- -run TestMigrateInOrderOnce ./internal/store  # one Go test
task test:web -- src/lib/sync                                 # web tests under one path
task testdata                                                 # make tiny test clips with ffmpeg (slice 5)
task lint                                                     # golangci-lint (.golangci.yml), svelte-check
task build                                                    # build the production image
task up | task down | task logs                               # build + start, stop, follow the production stack
```

Ask before `task up` or `task down`: they restart the live server, maybe mid-movie. Dev and prod use
different ports, compose project names and data dirs, so `task dev` is safe to run next to prod.

Prod config lives in `.env` (template: `.env.example`). Each variable arrives with the slice that first
uses it:
- `MEDIA_ROOT`: host folder holding all media, mounted read-only at `/media` (slice 5).
- `DATA_DIR`: host folder for the database, cache and backups, mounted at `/data` (slice 2). Required.
  `task up` creates it, owned by you. Compose itself uses `create_host_path: false`, so a plain
  `docker compose up` with a missing folder fails instead of Docker making it as root, where the app
  can't write.
- `PUBLIC_BIND`: host IP for the guest port, default `127.0.0.1`. Nothing is open to guests until you set
  it to the host's Tailscale IP.
- `GUEST_PORT`, `ADMIN_PORT`: host ports, default `8420` and `8421`. Inside the container Go always
  listens on `8080` and `8081`. Only the host side changes, so a clash on the host (8080 is a popular
  port) never touches code or tests.

`.env` is for prod only. `compose.dev.yml` hardcodes dev's ports and data dir (`./.dev-data`, via Go's
`DATA_DIR` env inside the repo mount, so Go creates it as you; the database is
`./.dev-data/pausetogether.db`). Dev reads the same `MEDIA_ROOT` as prod (from slice 5); it's mounted
read-only, so sharing it is safe. Go reloads with `air` inside the dev container (`.air.toml`). Dev
publishes only `127.0.0.1:5173`; Go's ports stay inside the Docker network. Tests and lint run in the dev
containers (`compose run --rm --no-deps`).

- Containers run as you (`HOST_UID`/`HOST_GID`, set by the Taskfile), so nothing root-owned lands in the
  repo. Not `UID`: shells treat it as read-only.
- Dev caches (Go build, modules and `GOPATH`, npm, air) live in `./.cache/`, ignored by git and Docker.
- `npm ci` runs only when `web/package-lock.json` is newer than `web/node_modules/.package-lock.json`,
  so it never wipes `node_modules` under a running dev server.
- Go reads `GUEST_ADDR`, `ADMIN_ADDR`, `DATA_DIR` and `TOKEN_COOKIE` (defaults `:8080`, `:8081`,
  `/data`, `pt_token`). Dev sets `TOKEN_COOKIE=pt_token_dev`: cookies ignore the port, so dev
  (`localhost:5173`) and prod (`localhost:8421`) would otherwise overwrite each other's user.
- `kill_delay` in `.air.toml` (6 s) must stay longer than `main.go`'s 5 s shutdown timeout, or reloads
  skip the clean shutdown.
- Vite proxies only `^/api/`, `^/stream/` and `^/ws`. A new Go URL prefix needs its own entry in
  `web/vite.config.ts`.
- The Dockerfile's Go stage copies only `cmd/`, `internal/` and `web/embed.go`. A new top-level Go folder
  must be added there, or the prod build fails while dev still works.

## Layout

```
cmd/pausetogether/   main: config, wiring, both HTTP listeners
internal/api/        HTTP handlers, guest vs admin routes, SPA serving
internal/room/       (planned) room state, sync engine, presence, chat, WebSocket hub
internal/library/    (planned) folder scanning, Plex name parsing, file watching
internal/media/      (planned) ffprobe, prepare jobs, subtitles to WebVTT, cache
internal/store/      SQLite open, migrations (migrations/*.sql embedded, applied at startup), backups
internal/user/       name rules, users table (token stored as a SHA-256 hash), lookup by cookie token
web/                 SvelteKit app; web/embed.go embeds its build (go:embed can't reach ../)
Dockerfile           web build → Go build → runtime image (ffmpeg from slice 5); go-dev stage for dev
compose.yml          production
compose.dev.yml      development
Taskfile.yml
```

URL prefixes: `/api` JSON, `/api/admin` admin only, `/ws` WebSocket (one per open room page), `/stream`
prepared videos and subtitles. The URL prefix is `/stream`, not `/media`, on purpose: `/media` is the
container mount for source files, and one name for two things gets mixed up in code.

- Unknown `/api/*` and `/stream/*` paths return 404. Only other paths fall back to `index.html`.
- `index.html` is served with `Cache-Control: no-cache`, `_app/immutable/*` as immutable.
- The server sends its build ID when a socket connects. If it doesn't match the page's, the page
  reloads. So after a deploy, open tabs never run old JS against the new server.

## Access and networking

- One process, two HTTP listeners:
  - **Guest port** (host `8420`, container `8080`): the whole app except the admin API. Guests reach it over Tailscale.
  - **Admin port** (host `8421`, container `8081`): the same app **plus** the admin API. Compose publishes it on `127.0.0.1`
    only, so only the host machine can reach it. The host uses `http://localhost:8421` for everything.
- Why ports and not IP checks: inside Docker, the host's own requests arrive from the Docker network
  gateway, not `127.0.0.1`, so the app can't tell host from guest by IP. Never add IP-based admin checks.
- Tailscale runs on the host OS, not in a container, and it is the only access control: anyone who
  reaches the guest port is trusted. Docker bypasses `ufw`, so a port bound to `0.0.0.0` is open on
  every network the host joins (café Wi-Fi too). That's why `PUBLIC_BIND` defaults to `127.0.0.1`.
- Set `net.ipv4.ip_nonlocal_bind=1` on the host. At boot, Docker can start before Tailscale has its IP.
  Without this, binding the Tailscale IP fails with "cannot assign requested address".
- Plain HTTP over the tailnet (WireGuard already encrypts). Guest pages are **not** a secure context:
  no `crypto.randomUUID()`, `navigator.clipboard`, Wake Lock or other HTTPS-only browser APIs.
- Wrap state-changing routes in `http.CrossOriginProtection` (Go 1.25+, CSRF), above all on the admin
  port.
- The admin listener rejects any Host header that isn't `localhost`, `127.0.0.1` or `[::1]`, compared
  without the port (browsers send `localhost:8421`). This stops DNS rebinding: a web page pointing its
  own domain at `127.0.0.1` looks "same origin", so `CrossOriginProtection` sees nothing wrong.
- Keep coder/websocket's Origin check on. Never `InsecureSkipVerify`.
- In dev, Vite proxies `/api`, `/ws` and `/stream` to Go's admin listener, so `localhost:5173` is the
  host's view (`isAdmin` true), like `localhost:8421` in prod. The guest view is covered by tests and
  seen on the prod image's guest port. Don't set Vite's `changeOrigin`: it rewrites Host to the Docker
  service name, and the Host check then rejects every admin call. Vite's string shorthand
  (`'/api': url`) turns `changeOrigin` on, so proxy entries are objects with `changeOrigin: false`.
- Bandwidth: every viewer streams the original-quality file. The host's upload must cover
  bitrate × viewers, and a guest on a relayed (DERP) Tailscale link may buffer, which pauses the room.
- Host offline: open tabs show "Host is offline, reconnecting…" and keep retrying. A fresh visit just
  fails; fixing that needs a second machine, which is out of scope.

## Users

- No accounts: one host (anyone on the admin port) plus anonymous guests.
- First visit: pick a display name. The server creates a user with a random token in an `HttpOnly`
  cookie (it rides along on the WebSocket upgrade too). The token keeps the name across visits.
- The cookie gets the longest life browsers allow (Chrome caps it at 400 days) and is refreshed on
  every visit.
- Cookies are per host name: `localhost`, the Tailscale IP and the MagicDNS name each give a different
  user. Give guests one address to use.
- Names: 1–32 runes, trimmed, at least one letter or number. No control characters, no invisible
  format characters (zero-width, text direction), no Unicode line breaks, no emoji. Emoji live in
  Unicode's "other symbol" class with `♥ ★ ©`, so those go too. Rename from a menu. Duplicates are
  allowed (no accounts, so no way to reclaim a name).
- `GET /api/me` returns `{name, isAdmin}` (`name` is null for a new visitor) and refreshes the cookie.
  The admin listener sets `isAdmin`. The page hides admin links when it's false.
- `POST /api/me {name}` renames a known user, or creates one (and sets the cookie) when the token is
  missing or unknown.
- The cookie (`pt_token`, dev `pt_token_dev`) is `SameSite=Lax` and never `Secure`: guests use plain HTTP, and their
  browser would drop a `Secure` cookie. `localhost` counts as secure, so dev would hide that bug.
- Everyone can: see all rooms, create and name rooms, control playback, switch a room's video, chat,
  archive and unarchive rooms.
- Host only (admin API): manage libraries (folder + type), rescan, delete rooms, set language defaults.
- Render all user text (names, chat) as text. Never `{@html}`.
- UI is English only, with all strings in one file so Turkish is easy to add later.

### Admin page

- Libraries: add, remove, rescan with progress.
- Files we can't use (skipped or unplayable), each with its reason, plus "Apple devices only" warnings.
- The job queue, with failed jobs and ffmpeg's error.
- Cache size and free disk space.
- Language defaults for new picks.
- Room delete is not here. It lives on the homepage room cards, shown only when `isAdmin`. The call
  still goes to `/api/admin`.

## Library

- Local files only. A library is a folder under `/media` plus a type. Never write into media folders.
- Libraries can't overlap: refuse a folder that equals, contains or sits inside another library's
  folder. Otherwise one file gets two identities.
- The admin folder picker browses `/media` through Go's `os.Root`, so a symlink can't lead outside it.
  The scanner doesn't follow directory symlinks (they can loop).
- Types: **Movies**, **TV Shows**, **Other Videos**.
- Video files are an allowlist of extensions: `.mkv .mp4 .m4v .mov .avi .webm .ts .m2ts`. Skip hidden
  files and macOS `._*` files.
- Plex naming is required for Movies and TV Shows. Files that don't match are skipped and listed on the
  admin page with the reason:
  - Movies: `Title (Year)/Title (Year).ext`; a loose `Title (Year).ext` is fine too.
    - `{edition-Director's Cut}` is kept as an edition label, so two editions in one folder stay
      apart. Other `{...}` tags are ignored.
    - Extras folders (`Featurettes`, `Behind The Scenes`, `Trailers`, `Extras`) and samples
      (`sample.mkv`) are skipped quietly, not listed.
    - Split files (`pt1`, `cd1`) are skipped with a reason.
  - TV: `Show (Year)/Season 01/Show (Year) - s01e02 - Episode Title.ext`; year and episode title optional.
    - Also accepted: two episodes in one file (`s01e01-e02`), specials (`Specials` or `Season 00`),
      season folders without a zero (`Season 1`), and episodes loose in the show folder.
    - Date-based (`Show - 2024-05-01`) and absolute-numbered (anime) episodes are skipped with a reason.
  - Other Videos: the file name is the title. Sub-folders become groups in the picker.
- Metadata comes from names only: no online lookups, no artwork. `ffprobe` supplies only technical
  facts (duration, codecs, audio and subtitle tracks).
- Sidecar subtitles sit next to the video with Plex names. Accepted: a 2- or 3-letter language code
  (`Title (Year).en.srt`, `.eng.srt`), the `forced`, `sdh` and `hi` flags (`.en.forced.srt`,
  `.en.sdh.srt`), and no language at all (`Title (Year).srt`). A `Subs/` folder is not read.
- A video's identity is its library + its path relative to the library folder. A rename makes a new
  video, and the old one turns missing.
- Video rows are never deleted, not even when their library is removed. Rooms and chat messages point
  at them. A gone file is marked missing, and its row keeps the title so old chat can still show it.
- Watching uses `fsnotify`. inotify watches one directory at a time, so add a watch per directory.
  Debounce events. A file moved into place counts as done (most tools rename when finished). A file
  written in place is probed once it stops growing.
- Rescan fully once at startup, then on a timer and on the admin "Rescan" button. Events get missed
  (host off, network drives, inotify limits), and torrent clients may create full-size files up front.
  A rescan re-probes only files whose size or mtime changed, and retries every failed probe.
- Clients send IDs, never file paths. The admin folder picker is the one exception and must stay inside
  `/media`.

## Media pipeline

Browsers play only some codecs, and a remux streamed on the fly can't seek (no fixed length, no index).
So each room's video is **prepared once**, then served as a plain file.

- Codecs are checked at scan against an allowlist: H.264 (8-bit 4:2:0 only), HEVC, AV1, VP9.
  Everything else is marked unplayable (VP8, 10-bit or 4:2:2/4:4:4 H.264, MPEG-2, VC-1, XviD, …).
  Dolby Vision profile 5 gets an "Apple devices only" warning: other screens show it purple and green.
- The scan also builds the full codec string from ffprobe data (e.g. `avc1.640028`) and sends it with
  the video. The client checks that with `canPlayType()`: a bare `hvc1` answers "maybe" to almost
  anything. The `<video>` error event is the backstop. Video is never transcoded, so when a device can't
  decode it, show "This device can't play HEVC", not a black screen.
- Prepare = one ffmpeg run into `/data/cache`:
  - Video copied, never re-encoded (original quality). HEVC is tagged `hvc1` (`-tag:v hvc1`) or Apple
    devices refuse it.
  - Only the room's chosen audio track (browsers can't switch audio tracks). Copied if it's AAC with at
    most two channels. Otherwise converted to stereo AAC at 192k:
    - mono and stereo: just `-ac 2`.
    - more channels: normalize the layout first (7.1 → 5.1, side → back), then one center-boosting
      `pan` formula, then a light `acompressor` so explosions don't drown voices on phone speakers.
      ffmpeg's default 5.1 → stereo downmix makes voices quiet.
  - All embedded text subtitle tracks become WebVTT in the same run. Their packets are spread across the
    whole file, so pulling them out costs a full read, just like the remux.
  - `-movflags +faststart`, so the index sits at the front and playback starts right away.
  - Output goes to `*.tmp` and is renamed when done, so a half-written file never looks finished. Name
    the format (`-f mp4`), since ffmpeg guesses it from the extension. On start, delete leftover
    `*.tmp` files.
- Every video goes through prepare, even ones a browser could play as-is. One code path, on purpose.
- The cache key is video + audio track + source size and mtime + a recipe version number. Rooms with the
  same key share one copy. Bump the recipe version when the ffmpeg arguments change.
- Prepare is lazy: a job is queued when someone opens a room, or picks a video in one, and its copy is
  missing.
- Jobs run one at a time and report progress to the room, or its place in line ("Queued, 2nd in
  line"). A job is cancelled as soon as no room needs its result (switched away, archived, deleted).
- Before each job, check free disk space. If it's too low, fail with a clear message.
- A copy is deleted once nobody has opened any room using it for 7 days. Archived rooms don't count:
  they can't play. Opening the room prepares it again.
- A job reads and writes as fast as the disk allows, often on the disk viewers stream from. Measure
  first. If viewers buffer during a job, cap it with ffmpeg's `-readrate`.
- Serve with `http.ServeContent`: Range requests give native `<video>` seeking. No HLS.
- Subtitles: text tracks (embedded SRT/ASS/mov_text; sidecar `.srt`/`.vtt`/`.ass`) become WebVTT in the
  cache, in UTF-8. Embedded ones come out during prepare. Sidecars are converted at scan (they're
  small). ASS styling is lost. Image subtitles (PGS, VobSub) can't become text without OCR: list them
  as unavailable.
- Sidecar text encoding: valid UTF-8 or a BOM → use it. Otherwise pick the code page from the language
  in the file name (`tr` → Windows-1254, `ru` → Windows-1251, `el` → Windows-1253). Only then fall back
  to a detector (they often guess wrong on short files).
- The audio track is picked with the video and never changes after. A new track would mean a full
  re-prepare. Subtitle and subtitle offset can change any time. All three are room state, shared by
  everyone. Subtitles render in our own overlay, not native captions, so the offset is a simple time
  shift and we control where the text sits (clear of chat toasts and controls).
- If the video is gone (source moved or deleted, and no prepared copy), the room shows "Video missing"
  with the library picker. The new pick resumes at the same position.

## Rooms

- Anyone creates a room by picking a video. The picker asks for the audio track and subtitle on every
  pick, room creation and switch alike, with the defaults preselected.
- Defaults come from an admin setting: preferred audio language (or "original") and subtitle languages
  in order (e.g. `tr`, then `en`). Fall back to the file's default-track flag. Forced subtitles are
  turned on when their language matches the audio.
- Rooms have an optional name. Without one, show the current video.
- The homepage lists every room with its current video and who's watching. It polls `GET /api/rooms`
  every 5 s while the tab is visible, and right away when it becomes visible again.
- Rooms live until the host deletes them. Then the server sends "room deleted" to everyone in it, and
  their page goes home with a short note.
- Archive is allowed only when nobody is watching. An archived room can't play, and its chat is
  read-only. Archived rooms sit in their own homepage section and can be unarchived.
- A room plays one video at a time. Anyone can switch it with the library picker, and TV episodes also
  get a "Next episode" button, which never lands on a special. A switch starts at 0:00; only the "Video
  missing" swap keeps the position.
- Before a switch, the switcher confirms: "You're at 1:40:00. Switch to …?" Nobody else is asked.
- At the end of a video, the server (it knows the duration) pauses the room there. TV episodes show
  "Next episode". No autoplay.
- Progress is per room only: the position is saved on pause, seek and switch, and every 5 s while
  playing. No per-user progress.
- Presence: "watching now" (open socket) and "was here" (joined before, gone now), with a 15 s
  reconnect grace so flaky phones don't flicker between the two. Each user shows once, even with two
  tabs open.

## Sync

- **The server is the source of truth.** Room state: video, audio track, playing, position + server
  timestamp, subtitle, subtitle offset. Clients send intents (play, pause, seek, switch, subtitle,
  offset) and status (ready, buffering, away, can't play). The server applies them and broadcasts the
  full state.
- Status is tracked per socket, not per user. Clients send their position with each status message,
  and every few seconds while playing. That's how the server knows when a skipped client has caught up,
  and can show "Alice is 3 s behind".
- In Go, one goroutine per room owns its state. Everything reaches it through a channel. The pure sync
  logic runs inside that loop. No locks.
- Server time is milliseconds since server start (monotonic), so it restarts at 0. Wall time is only
  for what gets stored.
- Clients estimate their clock offset to the server by ping/pong, keeping the lowest-RTT sample of the
  last 10 pings (clocks drift, so old samples go stale). They throw the offset away on every reconnect
  and the first pong gives a new one. After a restart every room is paused, so nothing needs the offset
  until someone presses play. Clients time with `performance.now()`, never `Date.now()` (it can jump).
- The clock ping, sent every few seconds, doubles as the heartbeat. No ping for 10 s → the socket
  counts as disconnected. The client reconnects with backoff and gets the full state.
- Drift: ignore it under 200 ms. Up to 1 s, nudge `playbackRate` by 5–10%. Beyond that, seek.
- Play, pause and seek apply locally at once, then snap to the server's state when it arrives. The seek
  bar sends a seek only when the user lets go.
- Only our own controls send intents. `<video>` events (a `pause` from a locked phone, `waiting`) are
  status, never commands.
- The room waits for everyone. A client buffering or away (tab hidden or backgrounded) for longer than
  3 s pauses the room ("Waiting for Alice"). It resumes when everyone is ready. Anyone can press "Play
  anyway": the blocking client is then skipped until it catches up or comes back.
- Some clients never block, and none of them count as away:
  - a device marked "can't play" (it can't decode the video),
  - someone who hasn't pressed "Tap to join" yet,
  - a new joiner, until it has been ready once.
- When someone pauses, the player shows a small note for 2 s ("Alice paused"). Not in chat, not stored.
- The room pauses when the last person leaves. On server start, every room loads paused.
- Timing numbers in this spec are named constants, kept in one place per side.
- Keep sync logic pure (no IO, clock injected) on both sides, `internal/room` and `web/src/lib/sync`,
  so it can be unit-tested.

## Player

- `<video playsinline>`, or iPhone opens its own fullscreen player on play: no chat, no subtitles, none
  of our controls. Keep one `<video>` element for the life of the room page and only change its `src`;
  a new element may need a fresh tap on iOS.
- No native `controls` attribute. Set `disableRemotePlayback` and `disablePictureInPicture`: AirPlay,
  Chromecast and Picture-in-Picture take the video out of the page, away from our subtitles, controls
  and sync. Not in v1.
- Browsers block autoplay with sound, so entering a room shows a "Tap to join" button first.
- Fullscreen the player wrapper, not the `<video>`, so chat and subtitles stay on top. Where
  `document.fullscreenEnabled` is false (iPhone Safari), fill the window with CSS instead.
- No playback speed control. `playbackRate` belongs to the drift fix.
- Per device, in `localStorage`: volume, mute, subtitle size. Everything in room state is shared.
- Subtitle cues: `<i>` and `<b>` become real elements. Everything else (`{\an8}`, ASS tags, other
  markup) is dropped.
- Library picker: a search box, show → season → episode grouping, and a "recently added" sort. Videos
  this device can't play are greyed out.

## Chat

- One chat per room, kept forever, deleted with the room (foreign-key cascade).
- Plain text, at most 1000 characters. Emoji are ordinary Unicode typed on the device keyboard; each
  device draws its own.
- Live messages go both ways on the room socket. Opening the room loads the last 100; older ones load
  on scroll up (`GET` with a cursor).
- A message can reply to one earlier message and shows a short quote of it.
- People can delete their own messages, not edit them. A reply to a deleted message quotes "deleted
  message".
- Each message stores the video and the room position when it was sent (from the server's room clock)
  and shows that as a timestamp, plus the video's name if it isn't the one playing now. The send time
  (wall clock) shows on tap or hover.
- No typing indicator, no sounds, no system messages: play, pause, seek, switch, join and leave never
  appear in chat.
- UI: a side panel on wide screens, a bottom sheet on portrait phones. While it's closed, new messages
  pop up as small toasts over the video and fade out; tapping one opens a reply to it. Must work in
  fullscreen.

## Operations

- Start at boot: `restart: unless-stopped` in `compose.yml`, and the Docker service enabled in systemd.
  The host PC is restarted often.
- Clean shutdown on SIGTERM (`task down`, or the PC shutting down): save room positions, close sockets,
  stop ffmpeg and delete its temp file. Set `stop_grace_period` in compose so the save has time to
  finish; Docker kills the process after 10 s by default.
- Backups: `VACUUM INTO` a file in `/data/backups` on start, then every 24 h while up. Skip it if the
  newest backup is under 24 h old (checked hourly, so a restart never stretches the gap to 48 h). The
  time is in the file name. Keep the last 7. Never copy the live database file: with WAL, a
  plain copy can be broken.

## Conventions

- Write unit tests for new logic: Go table-driven tests next to the code, Vitest for `web/src/lib`. The
  Plex name parser gets plenty of real-world file names. No end-to-end tests for now.
- The ffmpeg arguments are where the bugs will be. So a few Go tests may run real ffmpeg inside Docker,
  on the tiny clips `task testdata` makes: Plex names, 5.1 and 7.1 audio, a Windows-1254 `.srt`, an
  HEVC file.
- SQLite: every connection sets `foreign_keys=ON` (off by default, and cascades silently don't run
  without it), WAL and `busy_timeout`, via `_pragma` in the DSN. Also `_txlock=immediate`: a
  transaction that reads, then writes, can otherwise fail with `SQLITE_BUSY` at once, without waiting.
- Migrations: `NNNN_name.sql` files in `internal/store/migrations`, numbered 1, 2, 3… with no gaps,
  tracked with `PRAGMA user_version`. One transaction each. Never edit an applied migration; add a new
  one. The folder keeps a `.gitkeep` (embedded with `all:`) so it compiles while empty.
- Migrations run on a dedicated connection with `foreign_keys=OFF`, set outside the transaction (inside
  one it does nothing). Rebuilding a table (make new, copy, drop old) would otherwise run
  `ON DELETE CASCADE` on the drop, and rebuilding `rooms` would wipe every chat message. Run
  `PRAGMA foreign_key_check` before commit. Turn `foreign_keys` back `ON` before the connection returns
  to the pool.
- WebSocket messages are JSON `{"type": ..., ...}`, defined once per side (`internal/room/protocol.go`,
  `web/src/lib/protocol.ts`). Change both together.
- The frontend uses relative URLs only, so one build works on both ports and behind the dev proxy.
- LF line endings everywhere.
