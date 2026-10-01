# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

**PauseTogether**: *"Distance can't pause us."* A self-hosted watch-together web app. One host machine
serves its local video library. Guests reach it over Tailscale and watch in sync from PCs, tablets and
phones. Same show. Same second. Different places.

**Status: slices 1–19 are built; the field test is next (`docs/plan.md`).** This file is the spec. When
code changes, update the layout and commands below to match reality. The rules here are decided: flag problems,
but don't quietly change them. Work each slice by the steps under "How to work a slice" in
`docs/plan.md`.

## Stack

- **Backend:** Go, standard library first. `net/http` routing with method/path patterns
  (`GET /api/rooms/{id}`), no web framework. `database/sql` with hand-written SQL, no ORM. `log/slog`.
  WebSockets via `github.com/coder/websocket`. SQLite via `modernc.org/sqlite` (pure Go, so
  `CGO_ENABLED=0` and simple Docker builds). File watching via `github.com/fsnotify/fsnotify`.
  Subtitle code pages via `golang.org/x/text`.
  `ffmpeg`/`ffprobe` run through `os/exec`.
- **Frontend:** SvelteKit with `adapter-static` in SPA mode (`fallback: 'index.html'`, `ssr = false` in
  the root layout), Svelte 5 runes, Tailwind v4 (config lives in CSS via `@theme`; there is no
  `tailwind.config.js`). Go serves real files and falls back to `index.html` for app routes like
  `/rooms/{id}`.
- **Look:** design tokens live in `web/src/app.css` under `@theme`: six named colors (`midnight` page,
  `dusk` raised surfaces, `haze` quiet text, `moonlight` text, `lamp` accent, `ember` delete and errors),
  plus `line` (borders) and plain `black` and `white` (the video box, subtitles). Tailwind's own colors
  and text sizes are cleared, so only these exist: `text-sm` to `text-3xl`, and a `text-xs` or
  `neutral-700` silently builds to nothing. Radii add `rounded-control` and `rounded-panel` to
  Tailwind's own; `font-display` is for titles. Shared classes there too (`.btn` with `-primary`,
  `-quiet`, `-danger`, `-small`; `.field`, `.icon-btn`, `.row`, `.pill`). Components use the tokens, never loose hex values. Fonts are
  bundled with `@fontsource-variable` (Big Shoulders for titles, Atkinson Hyperlegible Next for the
  rest), never loaded from a CDN: guests may have no route to the internet. Controls are at least
  44 px on touch screens.
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
task test:web -- src/lib/me                                   # web tests under one path (a wrong path passes with 0 tests)
task testdata                                                 # tiny test clips (testdata/media); task test:go runs it first
task lint                                                     # golangci-lint (.golangci.yml), svelte-check
task build                                                    # build the production image
task up | task down | task logs                               # build + start, stop, follow the production stack
```

Ask before `task up` or `task down`: they restart the live server, maybe mid-movie. Dev and prod use
different ports, compose project names and data dirs, so `task dev` is safe to run next to prod.

Prod config lives in `.env` (template: `.env.example`):
- `MEDIA_ROOT`: host folder holding all media, mounted read-only at `/media`. Required. The
  folder must exist: compose won't make it (`create_host_path: false`).
- `DEV_MEDIA_ROOT`: optional, dev only, e.g. a scratch folder to copy test files into. Also works from
  the shell: `DEV_MEDIA_ROOT=/tmp/media task dev`.
- `DATA_DIR`: host folder for the database, cache and backups, mounted at `/data`. Required.
  `task up` creates it, owned by you. Compose itself uses `create_host_path: false`, so a plain
  `docker compose up` with a missing folder fails instead of Docker making it as root, where the app
  can't write.
- `PUBLIC_BIND`: host IP for the guest port, default `127.0.0.1`. Nothing is open to guests until you set
  it to the host's Tailscale IP.
- `GUEST_PORT`, `ADMIN_PORT`: host ports, default `8420` and `8421`. Inside the container Go always
  listens on `8080` and `8081`. Only the host side changes, so a clash on the host (8080 is a popular
  port) never touches code or tests.

## Dev setup

Dev reads only `MEDIA_ROOT` and `DEV_MEDIA_ROOT` from `.env`. The rest of `.env` is prod only.

- `compose.dev.yml` hardcodes dev's ports and data dir: `./.dev-data`, set through Go's `DATA_DIR`
  env inside the repo mount, so Go creates it as you. The database is `./.dev-data/pausetogether.db`.
- Dev mounts `DEV_MEDIA_ROOT` if set, else prod's `MEDIA_ROOT`, read-only at `/media`, so sharing it
  with prod is safe. Compose checks for one of the two on every command, so `task dev`, `task test`
  and `task lint` all fail until one is set.
- By-hand checks that add, rename or delete media use `DEV_MEDIA_ROOT` pointed at a scratch folder,
  never the real library.
- Go reloads with `air` inside the dev container (`.air.toml`).
- Dev publishes only `127.0.0.1:5173`. Go's ports stay inside the Docker network.
- Tests and lint run in the dev containers (`compose run --rm --no-deps`).
- Run Go, npm and tests through `task`, never the host's `go` or `npm`: versions and ffmpeg match
  the images only in Docker.
- Containers run as you (`HOST_UID`/`HOST_GID`, set by the Taskfile), so nothing root-owned lands in the
  repo. Not `UID`: shells treat it as read-only.
- Dev caches (Go build, modules and `GOPATH`, npm, air) live in `./.cache/`, ignored by git and Docker.
- `npm ci` runs only when `web/package-lock.json` is newer than `web/node_modules/.package-lock.json`,
  so it never wipes `node_modules` under a running dev server.
- Go reads `GUEST_ADDR`, `ADMIN_ADDR`, `DATA_DIR` and `TOKEN_COOKIE` (defaults `:8080`, `:8081`,
  `/data`, `pt_token`). Dev sets `TOKEN_COOKIE=pt_token_dev`: cookies ignore the port, so dev
  (`localhost:5173`) and prod (`localhost:8421`) would otherwise overwrite each other's user.
- Go also reads `PUBLIC_BIND` and `GUEST_PORT`, which `compose.yml` passes in from `.env`, to show the
  host the guest link (`http://{PUBLIC_BIND}:{GUEST_PORT}`). A loopback or `0.0.0.0` bind gives no link.
  Dev passes neither, so dev shows the "No guest link yet" note instead. IPv6 binds come in brackets.
- Go also reads `BUILD_ID`; without it, the ID comes from the embedded web build. Dev sets
  `BUILD_ID=dev` on both containers (`svelte.config.js` reads it too), or Vite's pages would never
  match Go's ID and would reload forever.
- `kill_delay` in `.air.toml` (6 s) must stay longer than `main.go`'s 5 s shutdown timeout, or reloads
  skip the clean shutdown.
- Vite proxies only `/api` and `/stream` (and paths under them), and `/ws` (exactly, plus a query
  string). A new Go URL prefix needs its own entry in `web/vite.config.ts`, shaped like the others: a
  regex key (`'^/api(/|$)'`, so `/streams` stays an SPA page) and an object value (why: see
  `changeOrigin` under "Access and networking").
- The Dockerfile's Go stage copies only `cmd/`, `internal/` and `web/embed.go`. A new top-level Go folder
  must be added there, or the prod build fails while dev still works.

## Layout

```
cmd/pausetogether/   main: config, wiring, both HTTP listeners
internal/api/        HTTP handlers, guest vs admin routes (admin API registered on the admin port only);
                     server.go (routes, both listeners), spa.go (SPA serving), host.go (admin Host check),
                     me.go (user cookie), rooms.go, videos.go (picker), admin.go (admin API),
                     ws.go (room socket join), stream.go (/stream files), guest.go (the guest link)
internal/room/       rooms (create, switch, rename, archive, delete) and the prepare jobs they need;
                     sync.go: one room's sync rules, pure (clock passed in); timing.go: its timing constants;
                     hub.go (sockets, one loop per room with people in it), loop.go (a room's goroutine),
                     presence.go (pure), protocol.go (socket messages); chat.go (messages: add, delete, pages)
internal/library/    Plex name parsing (pure: path in, video/subtitle/skip out), scanning into videos and tracks,
                     scan queue with progress, add/remove libraries, folder picker, file watching; the picker's
                     video list and details, language codes (normalized to 2 letters), language defaults;
                     a room's video and its prepare job
internal/media/      ffprobe and the codec check; prepare jobs (ffmpeg arguments, job queue, cache in DATA_DIR/cache);
                     sidecar subtitles to WebVTT (encoding, ffmpeg over stdin)
internal/store/      SQLite open, migrations (migrations/*.sql embedded, applied at startup), backups
internal/user/       name rules, users table (token stored as a SHA-256 hash), lookup by cookie token
testdata/make.sh     makes the test clips in testdata/media (git-ignored)
docs/plan.md         build order in slices, and how to work one
.claude/skills/      git-commit, fix-comments (used by the slice steps in docs/plan.md)
web/                 SvelteKit app; web/embed.go embeds its build (go:embed can't reach ../)
web/src/app.css      design tokens (@theme), shared classes, font imports
web/src/lib/         api.ts (fetch helper, shared API), admin.ts (admin API), me.svelte.ts (current user),
                     Brand.svelte (the app name with its pause mark: header, welcome),
                     picker.ts (pure picker logic: grouping, search, default audio and subtitle, next episode),
                     Picker.svelte, Dialog.svelte (modals on the browser's <dialog>: focus, Escape, inert page),
                     FolderPicker.svelte, NameForm.svelte, GuestLink.svelte (the guest link, host only), rooms.ts (pure room helpers), strings.ts,
                     protocol.ts (socket messages), socket.ts (room socket: ping, reconnect, build ID),
                     Player.svelte (the <video>, prepare progress, "Tap to join", controls, the follow loop, subtitle panel,
                     fullscreen, where the chat panel and toasts sit), subtitles.ts (pure: WebVTT cues, cue sanitizer, subtitle URL),
                     Subtitles.svelte (the subtitle overlay), prefs.ts (per-device player settings in
                     localStorage), time.ts (1:40:00), chat.ts (pure: message length, the message list),
                     Chat.svelte (the chat panel, with who's watching), Icon.svelte (inline SVG icons);
                     sync/ (pure): clock.ts (server clock offset), drift.ts (drift fix, follow step), state.ts
                     (target position, local intents), status.ts (what the player reports, and when),
                     timing.ts (its timing constants)
web/src/routes/      homepage (room list), rooms/[id] (room page), admin
web/static/          favicon.svg
README.md            for users: setup (Tailscale, ip_nonlocal_bind, Docker at boot), file naming
Dockerfile           web build → Go build → runtime image with Debian's ffmpeg; go-dev stage (also ffmpeg) for dev and tests
compose.yml          production
compose.dev.yml      development
Taskfile.yml
```

URL prefixes: `/api` JSON, `/api/admin` admin only, `/ws` WebSocket (one per open room page), `/stream`
prepared videos and subtitles, the key being the cache key: `/stream/{key}/video.mp4`, an embedded track
`/stream/{key}/{stream}.vtt` (ffprobe's stream index), a sidecar `/stream/{sidecarKey}/subtitle.vtt`
(its own key: file + size + mtime). The URL prefix is `/stream`, not `/media`, on purpose: `/media` is the
container mount for source files, and one name for two things gets mixed up in code.

- Unknown `/api/*` and `/stream/*` paths return 404. Only other paths fall back to `index.html`.
- `index.html` is served with `Cache-Control: no-cache`, `_app/immutable/*` as immutable.
- The server sends its build ID when a socket connects. If it doesn't match the page's, the page
  reloads. So after a deploy, open tabs never run old JS against the new server. The ID is SvelteKit's
  `version`: the page has it from `$app/environment`, Go reads it from `_app/version.json` in the build.

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
- In dev, Vite proxies to Go's admin listener, so `localhost:5173` is the
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
- First visit: pick a display name. The form says what the app is, and which room a room link leads to.
  The server creates a user with a random token in an `HttpOnly` cookie (it rides along on the WebSocket
  upgrade too). The token keeps the name across visits.
- The cookie gets the longest life browsers allow (Chrome caps it at 400 days) and is refreshed on
  every visit.
- Cookies are per host name: `localhost`, the Tailscale IP and the MagicDNS name each give a different
  user. Give guests one address to use: the host's pages show it (the guest link, below).
- Names: 1–32 runes, trimmed, at least one letter or number. No control characters, no invisible
  format characters (zero-width, text direction), no Unicode line breaks, no emoji. Emoji live in
  Unicode's "other symbol" class with `♥ ★ ©`, so those go too. Rename from a menu. Duplicates are
  allowed (no accounts, so no way to reclaim a name).
- `GET /api/me` returns `{name, isAdmin}` (`name` is null for a new visitor) and refreshes the cookie.
  The admin listener sets `isAdmin`. The page hides admin links when it's false.
- The admin listener also sends `guestUrl`, the address guests open, built from the guest port's bind
  IP. The host browses on `localhost`, so no URL they could copy works for a guest. The room page shows
  the host "Guest link: http://100.101.102.103:8420/rooms/15" with a Copy button (`localhost` is a
  secure context, so the clipboard works there), and the admin page shows the bare address, or, when
  `PUBLIC_BIND` is still loopback, how to open the guest port.
- `POST /api/me {name}` renames a known user, or creates one (and sets the cookie) when the token is
  missing or unknown.
- The cookie (`pt_token`) is `SameSite=Lax` and never `Secure`: guests use plain HTTP, and their
  browser would drop a `Secure` cookie. `localhost` counts as secure, so dev would hide that bug.
- Everyone can: see all rooms, create and name rooms, control playback, switch a room's video, chat,
  archive and unarchive rooms.
- Host only (admin API): manage libraries (folder + type), rescan, delete rooms, set language defaults.
- Render all user text (names, chat) as text. Never `{@html}`.
- UI is English only, with all strings in `web/src/lib/strings.ts` so Turkish is easy to add later.

### Admin page

In this order: what needs the host's hand first, settings last. Under the title: the guest link, or
how to open the guest port.

- Libraries: add, remove, rescan with progress. Adding sits behind a button (open while there is no
  library), guesses the type from the folder's name (`TV Shows` → TV Shows) and says each type's naming,
  and its button names the result ("Add TV Shows as TV Shows"). A library whose folder is gone says so in plain words
  (moved, renamed, a drive not mounted), not as the raw file error. An empty folder counts as gone
  while the library has videos that aren't missing: an unmounted drive leaves its mount point behind.
  So the scan never marks a library's last video missing; the host removes the library instead.
- Files we can't use (skipped or unplayable), grouped by reason with a count, so a hundred misnamed
  files are one line with the fix. ffprobe's own message is one click away. Plus "Apple devices only"
  warnings.
- The job queue, with failed jobs and ffmpeg's error.
- Language defaults for new picks.
- Cache clean-up: cache size and free disk space, a "Clear cache" button (asks first) that deletes
  every prepared copy (converted sidecar subtitles and a running job stay), and after how many days
  unused a prepared copy is deleted.
- The page polls every 5 s while the tab is visible (every 1 s during a scan), and right away when it
  becomes visible again: the file watcher and the timed rescan change things behind its back.
- Room delete is not here. It lives on the homepage room cards, behind "Manage", shown only when
  `isAdmin`. The call still goes to `/api/admin`.

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
  admin page with the reason. Reasons are codes (`movie-no-year`): `Reason` constants in
  `internal/library/parse.go`, turned into text in `web/src/lib/strings.ts`. Add a new one to both.
  - Movies: `Title (Year)/Title (Year).ext`; a loose `Title (Year).ext` is fine too.
    - `{edition-Director's Cut}` is kept as an edition label, so two editions in one folder stay
      apart. Other `{...}` tags are ignored.
    - Text after the year is kept as a version label (`Dune (2021) - 4K.mkv`), so two versions in one
      folder stay apart too.
    - Only the file name counts, so movies can also sit in collection folders.
    - Extras are skipped quietly, not listed, and so are their subtitles (Movies and TV only; in Other
      Videos a `Trailers` folder is a real group). Plex's full list: folders `Behind The Scenes`,
      `Deleted Scenes`, `Featurettes`, `Interviews`, `Scenes`, `Shorts`, `Trailers`, `Other`, `Extras`,
      `Sample(s)`; names ending `-trailer`, `-behindthescenes`, `-deleted`, `-featurette`, `-interview`,
      `-scene`, `-short`, `-other`, `-sample`; and `sample.mkv`. An extras folder counts only inside a
      movie folder (`Title (Year)`) or a show, so a collection or show named `Shorts` still works. A
      file with `s01e02` in its name is never an extra.
    - Split files (`pt1`, `part1`, `cd1`, `disc1`, `disk1`) are skipped with a reason. Not `dvd1`:
      `DVD9` is a source label.
  - TV: `Show (Year)/Season 01/Show (Year) - s01e02 - Episode Title.ext`; year and episode title optional.
    - Also accepted: two episodes in one file (`s01e01-e02`, `s01e01e02`, `s01e01-02`), specials (`Specials` or `Season 00`),
      season folders without a zero (`Season 1`), and episodes loose in the show folder. The show comes
      from its folder, the season and episode from the file name.
    - Date-based (`Show - 2024-05-01`) and absolute-numbered (anime) episodes are skipped with a reason.
  - Other Videos: the file name is the title. Sub-folders become groups in the picker.
- Metadata comes from names only: no online lookups, no artwork. `ffprobe` supplies only technical
  facts (duration, codecs, audio and subtitle tracks).
- Sidecar subtitles sit next to the video with Plex names. A 2- or 3-letter language code is required
  (`Title (Year).en.srt`, `.eng.srt`), then optional `forced`, `sdh` and `hi` flags (`.en.forced.srt`,
  `.en.sdh.srt`). No language (`Title (Year).srt`) is skipped with a reason. A `Subs/` folder is not
  read. `hi` is also Hindi's code: first, it's the language (`.hi.srt`); after a language, the flag
  (`.en.hi.srt`).
- A video's identity is its library + its path relative to the library folder. A rename makes a new
  video, and the old one turns missing.
- Video rows are never deleted, not even when their library is removed. Rooms and chat messages point
  at them. A gone file is marked missing, and its row keeps the title so old chat can still show it.
- Watching uses `fsnotify`. inotify watches one directory at a time, so add a watch per directory.
  Debounce events. A file moved into place counts as done (most tools rename when finished). A file
  written in place is probed once it stops growing.
  - An event doesn't touch rows itself: it queues a normal scan of its library, 2 s after the last
    event (at most 30 s after the first). An unchanged library costs a folder walk; ffprobe runs only
    for files whose last probe failed.
  - A file with no write for 10 s has stopped growing. Until then, every scan leaves it alone.
- Rescan fully once at startup, then every hour and on the admin "Rescan" button. Events get missed
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
- ffprobe and ffmpeg always get absolute paths. A relative `-x.mkv` or `concat:x.mkv` would be read as
  an option or a protocol.
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
  - If the run fails, it runs once more without subtitles, so one broken subtitle track never costs a
    video that plays fine. That copy has no embedded subtitle files: check the folder, not the track
    list, before offering one. The room's `prepare` message lists the ones the folder has.
  - Output goes to a `{key}.tmp` folder, renamed to `{key}` when done, so a half-written copy never
    looks finished, and every file of one run appears at once. Name the format (`-f mp4`), since ffmpeg
    guesses it from the extension. On start, delete leftover `*.tmp` folders.
- Every video goes through prepare, even ones a browser could play as-is. One code path, on purpose.
- The cache key is video + audio track + source size and mtime + a recipe version number. Rooms with the
  same key share one copy. Bump the recipe version (`RecipeVersion` in
  `internal/media/prepare.go`) when the ffmpeg arguments change.
- Prepare is lazy: a job is queued when someone opens a room, or picks a video in one, and its copy is
  missing.
- Jobs run one at a time and report progress to the room, or its place in line ("Queued, 2nd in
  line"). A job is cancelled as soon as no room needs its result (switched away, archived, deleted).
- Before each job, check free disk space. If it's too low, fail with a clear message.
- A copy is deleted once nobody has opened any room using it for a number of days the host sets on
  the admin page: 1 to 365, default 7, in the `cache_settings` table. Archived rooms don't count:
  they can't play. Opening the room prepares it again. Sidecar copies share the cache folder but belong
  to their `sidecar_subtitles` row, not to rooms: the scan deletes them when the file goes.
  - "Last used" is the copy folder's mtime, not `video.mp4`'s: `http.ServeContent` sends the file's
    mtime as `Last-Modified`. The folder is touched when the job finishes, when a room opens or switches
    to a ready copy, and every hour while a room with people in it runs. The clean-up runs hourly,
    not at start: a clock that is wrong at boot would make every copy look unused. Unarchiving a room
    queues its prepare, since its copy may be gone.
- A job reads and writes as fast as the disk allows, often on the disk viewers stream from. Measured
  on the host's SSD: the disk still reads about 900 MB/s during a 4K job, and a 4K viewer needs about
  3 MB/s. So no `-readrate`. Measure again if media moves to a slower disk (a USB drive, a NAS).
- Serve with `http.ServeContent`: Range requests give native `<video>` seeking. No HLS.
- Subtitles: text tracks (embedded SRT/ASS/mov_text; sidecar `.srt`/`.vtt`/`.ass`) become WebVTT in the
  cache, in UTF-8. Embedded ones come out during prepare. Sidecars are converted at scan (they're
  small). ASS styling is lost. Image subtitles (PGS, VobSub) can't become text without OCR: list them
  as unavailable.
- Sidecar text encoding: valid UTF-8 or a BOM → use it. Otherwise the language in the file name picks
  the code page (`tr`/`tur` → Windows-1254, `en`/`eng` → Windows-1252, `ru`/`rus` → Windows-1251, …).
  No detector: they guess wrong on short files, and can't tell Turkish from Western European. A
  language with no single legacy code page (Chinese: GBK or Big5; Hindi) must be UTF-8.
- A sidecar that can't be read or decoded, or that ffmpeg turns into no cues (it never fails on junk text), is
  listed on the admin page as `sub-unreadable`.
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
  - Subtitle: the first listed language with a full (not forced) track wins, even when it's the
    audio's language; plain before SDH. Only when no listed language matches: a forced track in the
    audio's language, then the file's default track, then off.
  - Language codes are compared after normalizing to 2 letters where one exists (`eng`, `en` → `en`;
    `ger`, `deu` → `de`), on the server.
- Rooms have an optional name, set on the room page. It follows the rules for user names; blank means
  none. Without one, show the current video.
- A deleted room's id is never reused (`AUTOINCREMENT`), so an old link or open tab can't lead to
  another room.
- The homepage lists every room with its current video, where it is in it ("Video missing" when it
  can't play: the file is gone and no prepared copy is left), and who's watching, rooms with people in
  them first. While someone watches, "Join" on the first such room is the main action and "Watch something"
  steps back. Archive and Delete wait behind "Manage". It polls `GET /api/rooms` every 5 s while the
  tab is visible, and right away when it becomes visible again.
- Rooms live until the host deletes them. Then the server sends "room deleted" to everyone in it, and
  their page goes home with a short note.
- Archive is allowed only when nobody is watching. An archived room can't play, and its chat is
  read-only. Archived rooms sit in their own homepage section and can be unarchived.
- A room plays one video at a time. Anyone can switch it with the library picker, and TV episodes also
  get a "Next episode" button, which never lands on a special. A switch starts at 0:00; only the "Video
  missing" swap keeps the position.
- Before a switch, the switcher confirms: "You're at 1:40:00. Switch to …?" Nobody else is asked. Not
  asked when nothing is lost: at 0:00, at the end, or the "Video missing" swap.
- At the end of a video, the server (it knows the duration) pauses the room there. TV episodes show
  "Next episode"; anything else (a film, a show's last episode) shows "The end" and "Watch something
  else". No autoplay.
- Progress is per room only: the position is saved on pause, seek and switch, and every 5 s while
  playing. No per-user progress.
- Presence: "watching now" only: an open socket, plus a 15 s reconnect grace so flaky phones don't
  flicker out of the list. A page that closes its socket cleanly (tab closed, or went to another page)
  skips the grace: it left on purpose. Someone gone past the grace is forgotten, and nothing is stored.
  No "was here": each browser has its own cookie, so one person on three browsers would stay listed
  three times forever. Each user shows once, even with two tabs open.

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
- Leaving (closing the tab, or going to another page) counts as away, so the room pauses 3 s later
  too. It waits until that person is back and ready on a new socket. "Play anyway" forgets them
  instead of skipping: they can't catch up. Closing one of two tabs doesn't count: the person is
  still there.
- Some clients never block, and none of them count as away:
  - a device marked "can't play" (it can't decode the video),
  - someone who hasn't pressed "Tap to join" yet,
  - a new joiner, until it has been ready once.
- When someone pauses, the player shows a small note for 2 s ("Alice paused"). Not in chat, not stored.
- The room pauses when the last person leaves, and forgets who left before. On server start, every
  room loads paused.
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
- Browsers block autoplay with sound, so entering a room shows a "Tap to join" button first ("Click to
  join" where the pointer is a mouse).
- While the copy is prepared, the player shows "Preparing… 42%" with a progress bar, or its place in line.
- Fullscreen the player wrapper, not the `<video>`, so chat and subtitles stay on top. Where
  `document.fullscreenEnabled` is false (iPhone Safari), fill the window with CSS instead.
- Controls fade after 3 s with no mouse move, touch or key while the video plays: the round buttons
  over the video always, the control bar in fullscreen only (there it lies over the video, so hiding it
  never resizes the video, and subtitles move up while it shows). Never while paused, the subtitle panel
  is open or the seek bar is held. On touch screens, a tap on bare video hides them.
- Over the video's middle: back 10 s, play or pause, forward 10 s. Hidden while "Tap to join", "Waiting
  for …", or the end of the video ("Next episode", "The end") hold the middle. Before the room's first
  word on the socket, the middle says "Getting the room ready…" (after a moment, so a quick connect
  never flashes it).
- No playback speed control. `playbackRate` belongs to the drift fix.
- Keys, as in other players: Space or K play and pause, ← and → skip 10 s, J and L skip 20 s,
  F fullscreen, C the subtitle panel, H chat, M mute. Never while typing or with a dialog open. The
  bar's tooltips name them. On the focused seek bar, the arrows skip 10 s too: its own 1 s step would
  seek the whole room once per press.
- Per device, in `localStorage`: volume, mute, subtitle size (default medium). Everything in room state is shared.
  The subtitle panel says which is which: subtitle and timing under "For everyone", size under "On this
  screen".
- Subtitle cues: `<i>` and `<b>` become real elements. Everything else (`{\an8}`, ASS tags, other
  markup) is dropped.
- Library picker: a search box, show → season → episode grouping, and a "recently added" sort. Videos
  this device can't play are greyed out.

## Chat

- One chat per room, kept forever, deleted with the room (foreign-key cascade).
- Plain text, at most 1000 characters. Emoji are ordinary Unicode typed on the device keyboard; each
  device draws its own.
- Sending a message on a touch screen closes the keyboard, so the video shows again.
- Live messages go both ways on the room socket. Opening the room loads the last 100; older ones load
  on scroll up (`GET` with a cursor).
- A message can reply to one earlier message and shows a short quote of it.
- People can delete their own messages, not edit them. A deleted message disappears from the list.
  Its row stays with the text wiped, so a reply to it quotes "deleted message".
- Each message stores the video and the room position when it was sent (from the server's room clock)
  and shows that as a timestamp, plus the video's name if it isn't the one playing now. The send time
  (wall clock) shows on tap or hover.
- No typing indicator, no sounds, no system messages: play, pause, seek, switch, join and leave never
  appear in chat.
- UI: open on entering the room. A side panel on wide screens. On portrait screens it sits under the
  video and fills the rest of the screen (the lower half in fullscreen), not a floating sheet. While
  it's closed, new messages pop up as small toasts over the video and fade out; tapping one opens a
  reply to it. Must work in fullscreen.
- The "watching now" list sits at the top of the chat panel, under its header, like a chat app's
  online list. At most two lines, then it scrolls, so it never pushes the messages off a phone. Not
  shown while the chat is closed.

## Operations

- Start at boot: `restart: unless-stopped` in `compose.yml`, and the Docker service enabled in
  systemd (README.md). The host PC is restarted often.
- Clean shutdown on SIGTERM (`task down`, or the PC shutting down): pause every room and tell its
  pages (or they play on alone and jump back when the server returns), save room positions, close
  sockets, stop ffmpeg and delete its temp file. Open video streams never go idle, so the HTTP servers
  wait at most 5 s for them, then cut them off. `stop_grace_period: 30s` in compose leaves room for
  all of it; Docker kills the process after 10 s by default.
- Backups: `VACUUM INTO` a file in `/data/backups` on start, then every 24 h while up. Skip it if the
  newest backup is under 24 h old (checked hourly, so a restart never stretches the gap to 48 h). The
  time is in the file name. Keep the last 7. Never copy the live database file: with WAL, a
  plain copy can be broken.

## Conventions

- Write unit tests for new logic: Go table-driven tests next to the code, Vitest for `web/src/lib`. The
  Plex name parser gets plenty of real-world file names. No end-to-end tests for now.
- The ffmpeg arguments are where the bugs will be. So a few Go tests may run real ffmpeg inside Docker,
  on the tiny clips `task testdata` makes: Plex names, 5.1 and 7.1 audio, a Windows-1254 `.srt`, an
  HEVC file. Missing clips fail these tests instead of skipping them, so they never pass by not
  running.
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
  `web/src/lib/protocol.ts`). Change both together. `internal/room/testdata/protocol.json` holds one of
  each message and both sides' tests read it, so a new message goes there too.
- The frontend uses relative URLs only, so one build works on both ports and behind the dev proxy.
- LF line endings everywhere.
- No attribution lines in commits or PRs: no `Claude-Session` link, no "Generated with Claude".
