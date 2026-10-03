# Build plan

`AGENTS.md` is the spec. This file is the order we build it in: small slices, each tested and reviewed
before the next one starts.

## How to work a slice

1. Start a fresh session (`/clear`). Say which slice you're on. Branch off `main`
   (`slice/01-skeleton`).
2. Plan mode: read the slice and the matching parts of the spec, propose files and tests. Wait for approval.
3. Write the tests first.
4. Build until `task test` and `task lint` pass.
5. Do the slice's "by hand" check.
6. Update `AGENTS.md` where it no longer matches the code (layout, commands, "planned"/"design only").
7. `git add -N .`, so new files show up in the diff. Nothing is staged: the commit step groups the files.
    1. Run `/code-review high`.
    2. Run `/security-review` on the uncommitted working tree when the slice touches ports, cookies, the admin API, 
       file paths or the WebSocket.
    3. Fix comments with the `fix-comments` skill.
8. Human reads the diff (`git diff`).
9. Tick the box below. Commit with the `git-commit` skill, then rebase the branch onto `main` and merge it with 
   `git merge --ff-only`, so history stays a straight line with no merge commit.

Rules:
- Stay inside the slice. Anything under "Not here" waits for its own slice.
- If the spec looks wrong, stop and ask. Don't quietly work around it.
- A slice that needs a table adds its own migration file. No table is made before a slice needs it.

## Slices

### [x] 1. Skeleton

**Goal:** a Go binary with both listeners serves an empty SvelteKit page, in Docker, with every `task`
command working.

- `go.mod` (`go 1.25`), `cmd/pausetogether`, config from env, `slog`.
- Guest listener `:8080`, admin listener `:8081`. `GET /api/health` on both.
- SPA serving: `//go:embed all:build`, `web/build/.gitkeep`, fallback to `index.html`, 404 for unknown
  `/api/*` and `/stream/*`, cache headers. The handler takes an `fs.FS`, so tests use a fake build.
- Admin Host check. `http.CrossOriginProtection` wrapper ready for state-changing routes.
- SvelteKit (adapter-static SPA, Svelte 5, Tailwind v4, Vitest). One strings file for UI text.
- `Dockerfile`, `compose.yml`, `compose.dev.yml` (air, Vite proxy to the admin listener without
  `changeOrigin`), `.env.example`.
- Dev ports are published on `127.0.0.1` only: Docker bypasses `ufw`, and Vite's port reaches the admin API.
- `.gitignore`: `.env`, `.dev-data/`, `node_modules/`, `web/build/` except `.gitkeep`. `.dockerignore`
  too: dev's cache in `.dev-data/` holds whole movies, and `task build` would send it all to Docker.
- Taskfile: `dev`, `test`, `test:go`, `test:web`, `lint`, `build`, `up`, `down`, `logs`.

**Done when (tests):**
- `/api/nope` and `/stream/nope` → 404. `/rooms/abc` → `index.html` with `Cache-Control: no-cache`.
- `_app/immutable/*` → immutable cache header.
- Admin listener: Host `localhost:8081`, `127.0.0.1`, `[::1]` pass. `evil.com`, `localhost.evil.com` → rejected.

**By hand:** `task dev` shows the page at `http://localhost:5173`. `task build`, then run the image and
open `:8420` and `:8421`. `docker compose ps`: `8421` on `127.0.0.1` only, `8420` on `PUBLIC_BIND`.
`git status` is clean after both (`web/build/.gitkeep` still there).

**Not here:** database, users, any real page.

### [x] 2. Database

**Goal:** SQLite opens with the right settings, runs migrations safely, and makes backups.

- `internal/store`: DSN with `foreign_keys=ON`, WAL, `busy_timeout`, `_txlock=immediate`.
- Migration runner over an `fs.FS` (so tests can pass fake migrations). `PRAGMA user_version`. Dedicated
  connection, `foreign_keys=OFF` outside the transaction, `foreign_key_check` before commit, back `ON`
  after.
- Backups: `VACUUM INTO /data/backups` on start and every 24 h, skip if newest is under 24 h, keep 7.
  Clock injected.

**Done when (tests):**
- Every pooled connection reports `foreign_keys=1`, including the one the migrations used.
- Migrations apply in order, once. A failing one rolls back and leaves `user_version` unchanged.
- A migration that rebuilds a parent table keeps the child rows.
- Backup: made on start, skipped when fresh, only 7 kept.

**By hand:** `task dev`, then see the database file and one backup in `./.dev-data`.

**Not here:** real app tables.

### [x] 3. Users

**Goal:** a visitor picks a name once and keeps it.

- `users` table. Random token in an `HttpOnly` cookie, 400-day life, refreshed on every visit.
- Cookie is `SameSite=Lax` and **not** `Secure`: guests use plain HTTP, so their browser would drop it.
  `localhost` counts as secure, so dev would hide that bug.
- Create user, rename, `GET /api/me` → `{name, isAdmin}`. `isAdmin` true only on the admin listener.
- Name rules: trimmed, 1–32 runes (not bytes), at least one letter or number, no control, format
  (zero-width, text direction) or line-break characters, no emoji. The input's `maxlength` counts
  UTF-16 units, not runes, so the server decides.
- Page: first-visit name picker, rename menu.

**Done when (tests):**
- Name rules, table-driven (emoji refused, Turkish letters, tabs, zero-width and direction marks,
  33 characters, spaces only).
- Cookie flags and max-age. Unknown token → treated as a new visitor.
- Same request on guest vs admin listener → `isAdmin` false vs true.
- Cross-origin POST is refused.

**By hand:** pick a name, reload, still there. A second browser is a different user.

**Not here:** rooms, admin links.

### [x] 4. Plex name parser

**Goal:** pure functions that turn a relative path into a movie, episode, other video, sidecar subtitle,
quiet skip, or skip with a reason.

- Movies, TV Shows, Other Videos rules from the spec, including editions, extras folders, samples,
  split files, two-episode files, specials, `Season 1`, loose episodes, date-based and absolute episodes.
- Extension allowlist, hidden and `._*` files.
- Sidecar subtitle names: language code (required), `forced`/`sdh`/`hi`.

**Done when (tests):** a big table of real-world file names, each with the expected result.

**By hand:** read the test table. It's the spec for this slice.

**Not here:** touching the disk, ffprobe.

### [x] 5. Scan and probe

**Goal:** scanning a library folder fills the database with videos, their tracks and whether they play.

- `libraries` and `videos` tables (plus tracks). Identity = library + relative path. Rows never deleted;
  gone files marked missing.
- Walk without following directory symlinks. ffprobe through `os/exec`, behind an interface so the scan
  tests can fake it.
- ffprobe and ffmpeg always get absolute paths. A relative `-x.mkv` or `concat:x.mkv` would be read as an
  option or a protocol.
- Codec allowlist, full codec string (`avc1.640028`, `hvc1…`, `av01…`, `vp09…`), Dolby Vision profile 5
  warning.
- Rescan re-probes only on size or mtime change, and retries failed probes. Full scan at startup.
- Dev and test images install Debian's `ffmpeg`, the same 7.1 as prod.
- `task testdata`: tiny clips (Plex names, 5.1, 7.1, HEVC, 10-bit H.264). Later slices add the clips they
  need. `task test` runs it first (skipped when the clips exist), so real-ffmpeg tests never skip quietly.

**Done when (tests):**
- Codec check and codec string, table-driven from ffprobe JSON samples.
- Scan of a temp library: right rows, skipped files with reasons, missing after delete, rename → new
  video + old one missing.
- Unchanged file is not re-probed. A failed probe is retried on the next rescan.

**By hand:** `task testdata`, then `task test`.

**Not here:** admin page, file watching, subtitle conversion.

### [x] 6. Admin: libraries

**Goal:** the host manages libraries from the admin page.

- `/api/admin/libraries`: add (folder + type), remove, rescan with progress.
- Remove marks the library row removed (new column) instead of deleting it: `videos.library_id` points
  at it with no `ON DELETE`. Re-adding the same folder brings the row back, so video ids stay.
- `MEDIA_ROOT` in `.env.example`, mounted read-only at `/media` in `compose.yml` and `compose.dev.yml`.
- Folder picker browses `/media` through `os.Root`. Overlapping libraries refused.
- Admin page: libraries, skipped and unplayable files with reasons, "Apple devices only" warnings.
- Admin links shown only when `isAdmin`.

**Done when (tests):**
- `/api/admin/*` → 404 on the guest listener.
- Picker refuses `..` and a symlink that points outside `/media`.
- Overlap: same folder, parent, child, a symlink to another library's folder → refused. Sibling → fine.
- Removing a library keeps its video rows, marked missing.

**By hand:** add a library at `http://localhost:5173` (admin), see videos and the skipped list.

**Not here:** job queue, cache size, language defaults.

### [x] 7. File watching

**Goal:** new, renamed and deleted files show up without a manual rescan.

- fsnotify, one watch per directory, new directories get watched. Debounce.
- Moved-in file = done. Written-in-place file probed once it stops growing.
- Rescan on a timer.

**Done when (tests):**
- Debounce and "stopped growing" logic, pure with an injected clock.
- Temp dir: add, rename, delete, new sub-folder → database follows.

**By hand:** copy a file into the media folder, see it on the admin page within seconds.

**Not here:** anything about playback.

### [x] 8. Prepare jobs

**Goal:** one ffmpeg run turns a video + audio track into a cached MP4 that `<video>` can seek.

- Argument builder (pure): copy video, `-tag:v hvc1` for HEVC, chosen audio only, AAC ≤2 channels
  copied, else stereo AAC 192k: mono and stereo just `-ac 2`; more channels get the spec's downmix
  (layout normalize, center-boost `pan`, `acompressor`). `-movflags +faststart`, `-f mp4`, output to
  `*.tmp` then rename.
- Cache key: video + audio track + size + mtime + recipe version. Shared between rooms.
- Queue: one job at a time, place in line, cancel when unneeded, free-space check, delete stale `*.tmp`
  at start.
- `/stream` serves prepared files with `http.ServeContent`. The URL carries an ID, never a path.
- Admin page: job queue with failed jobs and ffmpeg's error, cache size, free disk space.
- `task testdata` adds: stereo AAC, two audio tracks.

**Done when (tests):**
- Argument builder, table-driven per case.
- Real ffmpeg on testdata: 5.1 and 7.1 → stereo AAC, stereo AAC copied, only the chosen audio track
  kept, HEVC tagged `hvc1`, index before data (faststart).
- Cancel mid-job leaves no `*.tmp`. Too little free space → the job fails with a clear message.
- Range request returns 206 with the right bytes. Unknown or malformed `/stream` ID → 404.

**By hand:** none yet (rooms start jobs). The real-ffmpeg tests are the check.

**Not here:** subtitles, rooms, cache clean-up after 7 days.

### [x] 9. Subtitles

**Goal:** every text subtitle becomes UTF-8 WebVTT in the cache.

- Sidecars recorded as subtitle tracks and converted at scan. Encoding: UTF-8 or BOM → use it; else
  code page from the language in the file name; no code page known for it → `sub-unreadable`.
- Embedded text tracks extracted in the prepare run (bump recipe version).
- Image subtitles (PGS, VobSub) listed as unavailable.
- Served under `/stream`.
- `task testdata` adds: a Windows-1254 `.srt`, a BOM `.srt`, an `.ass`, a clip with an embedded SRT track (the
  stereo clip from slice 5 already has one).

**Done when (tests):**
- Windows-1254 `.srt` → correct `ş ğ ı İ`, named both `.tr.srt` and `.tur.srt`. BOM file. ASS → VTT
  with styling dropped.
- Real ffmpeg: embedded SRT comes out as VTT next to the MP4.

**By hand:** open a `.vtt` from `/stream` and read it.

**Not here:** showing subtitles in the player.

### [x] 10. Library picker and language defaults

**Goal:** anyone can find a video and choose its audio and subtitle.

- Picker: search, show → season → episode grouping, Other Videos grouped by sub-folder, "recently
  added" sort, greyed out when `canPlayType()` says no.
- Audio and subtitle choice with defaults preselected.
- Admin setting: preferred audio language (or "original"), subtitle languages in order. Fallback to the
  default-track flag. Forced subtitles on when their language matches the audio.

**Done when (tests):** default-pick logic, table-driven (Vitest or Go, wherever it lives).

**By hand:** open the picker, search, see sensible defaults.

**Not here:** rooms.

### [x] 11. Rooms

**Goal:** rooms exist, list on the homepage, and start a prepare when opened.

- `rooms` table. Create from the picker, optional name, switch video (resets to 0:00).
- `GET /api/rooms`, homepage polls every 5 s while visible and right away on return.
- Archive/unarchive, archived section. Delete on room cards (admin only, `/api/admin`).
- Opening a room, or picking a video in it, queues its prepare if the copy is missing.

**Done when (tests):**
- Deleting a room removes it; video rows stay.
- Archived room can't be switched, and opening it queues no job.
- Opening a room queues exactly one job, even when two rooms share a cache key.
- Switch away, archive or delete → the job is cancelled, unless another room still needs it.

**By hand:** create two rooms on the same video, see one job in the admin queue.

**Not here:** WebSocket, playback, who's watching on room cards, "nobody watching" check for archive
(slice 13).

### [x] 12. Sync logic (pure)

**Goal:** the rules of sync, with no IO, fully tested on both sides.

- Go `internal/room`: apply intents and status (per socket), wait for everyone, 3 s buffering/away pause,
  "Play anyway" skip until caught up, clients that never block, end-of-video pause, last one leaves →
  pause, "N s behind", when to save position.
- TS `web/src/lib/sync`: clock offset from lowest-RTT of last 10 pings, reset on reconnect, drift rules
  (<200 ms ignore, ≤1 s nudge `playbackRate`, else seek), apply locally then snap to server.
- All timing numbers as named constants, one place per side.

**Done when (tests):** one test per spec sentence in the sync section. Written as scenarios
("Alice buffers 4 s → room pauses, reason Waiting for Alice").

**By hand:** read the scenario list.

**Not here:** sockets, `<video>`.

### [x] 13. WebSocket and room loop

**Goal:** browsers join a room over a socket and see the same state.

- One goroutine per room owns state, fed by a channel, runs the slice 12 logic. No locks.
- `/ws`: Origin check on, cookie identifies the user, build ID sent on connect (page reloads on
  mismatch), full state on connect.
- Build ID: the same value baked into the web build and the Go binary. Dev uses one fixed ID on both
  sides, or the page reloads forever.
- `internal/room/protocol.go` and `web/src/lib/protocol.ts`.
- Heartbeat is the client's clock ping, a JSON message (browsers can't send WebSocket ping frames).
  10 s without one → disconnected. Client reconnects with backoff.
- Presence: watching now / was here, 15 s grace, one entry per user. `GET /api/rooms` shows who's watching.
- Prepare progress and queue place pushed to the room. "Room deleted" message.
- Position saved on pause, seek, switch and every 5 s while playing. Archive only when nobody is watching.
- The subtitle offset is saved too (`rooms` has no column for it yet) and loaded by `NewSync`.

**Done when (tests):**
- `httptest` + two socket clients: play from one reaches the other.
- Wrong Origin refused. Missed heartbeat → disconnected.
- Presence: two tabs → one entry. A drop shorter than 15 s doesn't flip to "was here".
- Archive refused while someone is watching. Delete → everyone in the room gets "room deleted".
- Protocol: a shared JSON fixture file decodes the same in Go and TS.

**By hand:** two browsers in one room, see presence and prepare progress.

**Not here:** the player.

### [x] 14. Player

**Goal:** people watch together.

- One `<video playsinline>` for the page's life, only `src` changes. No native controls,
  `disableRemotePlayback`, `disablePictureInPicture`.
- "Tap to join". Our own controls; seek sent on release. `<video>` events are status only. Hidden tab →
  away.
- Drift fix wired to the slice 12 logic. "Waiting for Alice", "Play anyway", "Alice is 3 s behind",
  "Alice paused" note.
- `canPlayType()` check + error event → "This device can't play HEVC".
- Volume, mute in `localStorage`. "Host is offline, reconnecting…".

**Done when (tests):** any new logic in `web/src/lib` has Vitest tests.

**By hand:** two browsers, then a PC and a phone. Play, pause, seek, lock the phone, bad Wi-Fi. The phone
needs the guest port: `task build`, then `task up` with `PUBLIC_BIND` set to the Tailscale IP. This is
also the first check off `localhost`, which counts as secure and hides HTTPS-only API bugs.

**Not here:** subtitles, fullscreen, switching.

### [x] 15. Player extras

**Goal:** the rest of the player.

- Subtitle overlay with shared track and offset; `<i>`/`<b>` kept, everything else dropped; subtitle
  size in `localStorage`.
- Fullscreen on the wrapper; CSS fill where `document.fullscreenEnabled` is false.
- Switch from the picker with the "You're at 1:40:00" confirm. "Next episode" (never a special).
  "Video missing" swap keeps the position.

**Done when (tests):**
- Cue sanitizer, table-driven.
- "Next episode" pick, table-driven: after a two-episode file, across a season end, never a special.
- "Video missing" swap keeps the position.

**By hand:** subtitles on phone and PC, fullscreen on iPhone, next episode at the end.

**Not here:** chat.

### [x] 16. Chat

**Goal:** chat per room, alongside the video.

- `messages` table (cascade on room delete). Max 1000 characters, plain text.
- Live over the room socket. Last 100 on open, older with a cursor on scroll up.
- Reply with a short quote. Delete own message; replies show "deleted message".
- Room position and video stored per message; wall time on tap/hover.
- Side panel on wide screens, bottom sheet on portrait phones, toasts over the video (tap to reply),
  works in fullscreen. Text only, never `{@html}`.

**Done when (tests):** length limit, cursor paging, delete-then-reply quote, cascade on room delete.
Deleting someone else's message is refused. An archived room's chat is read-only.

**By hand:** chat between two devices, in fullscreen too.

### [x] 17. Operations

**Goal:** it survives reboots, deploys and a full disk.

- SIGTERM: save positions, close sockets, stop ffmpeg, delete its temp file. `stop_grace_period`,
  `restart: unless-stopped`. Exec-form `ENTRYPOINT`, so the Go process gets the signal itself.
- Cache clean-up: copies unused for 7 days (archived rooms don't count). The host sets the days on the
  admin page (1 to 365).
- "Clear cache" on the admin page: a red button under the cache size, with a warning before it deletes
  every prepared copy. Converted sidecars and a running job stay.
- Measure a prepare job while someone streams. Add `-readrate` only if viewers buffer.
- Host setup steps in `README.md`: `net.ipv4.ip_nonlocal_bind=1`, Docker enabled in systemd, `PUBLIC_BIND`.

**Done when (tests):** shutdown saves positions and leaves no `*.tmp`; clean-up rule with an injected clock;
Clear deletes every copy but sidecars and `.tmp` folders.

**By hand:** `task down` mid-movie, `task up`, room is paused at the right spot. Reboot the PC: the app
comes back by itself and a guest can reach it. "Clear cache" asks first, then the cache size drops to
the sidecars.

### [x] 18. UI refinement

**Goal:** the app looks designed, not like bare Tailwind. Same behaviour, new look.

- Work it with the `frontend-design` skill. In plan mode, write the design plan first: a palette of 4–6
  named colors, the typefaces and their roles, and ASCII wireframes for the homepage, the room page
  (wide screen and portrait phone) and the admin page. Check the plan against the skill's list of
  generic defaults, and revise what reads like one. Wait for approval before any code.
- Design tokens live in `web/src/app.css` under `@theme`. Components use the tokens, not loose hex
  values or one-off sizes.
- Fonts are bundled into the build (e.g. `@fontsource`), never loaded from Google Fonts or another CDN.
  The app is self-hosted, and guests may have no route to the internet.
- Every page and part: name form, homepage room cards, room page (player, controls, "Tap to join",
  waiting and paused notes, subtitle panel, chat panel, bottom sheet, toasts), picker, folder picker,
  admin page. The area around the video stays dark.
- Quality floor: works at 360 px wide, portrait and landscape. Touch targets at least 44 px on phones.
  Visible keyboard focus. `prefers-reduced-motion` respected. Text contrast at least WCAG AA.
- Copy may be reworded, but only in `web/src/lib/strings.ts`. Plain words, sentence case.
- No behaviour change beyond the player additions below. No new API or socket messages. The player
  rules still hold: one `<video playsinline>`, no native controls, subtitles clear of toasts and
  controls, chat works in fullscreen. User text is still rendered as text, never `{@html}`.
- Player additions, all client side (the rules are in AGENTS.md under "Player"):
  - Over the video's middle: back 10 s, play or pause, forward 10 s. They send the existing seek, play
    and pause intents.
  - Controls fade after 3 s without a mouse move, touch or key while the video plays; in fullscreen
    the control bar lies over the video and fades too. A tap on bare video hides them.
  - A progress bar under "Preparing… 42%".
  - Sending a chat message on a touch screen closes the keyboard.

**Done when (tests):** the existing tests pass unchanged. A test that breaks means behaviour changed:
fix the UI, not the test. New logic in `web/src/lib`, if any, gets Vitest tests. `task lint` passes.

**By hand:** screenshots of every page in dev, before and after, at 360×800, a phone in landscape and a
desktop width. Room page with subtitles on, chat open and closed (toasts), and fullscreen. Then a
real phone on the guest port.

**Not here:** other new features, Turkish strings, artwork or posters (the spec rules out online lookups).

### [x] 19. Presence in the chat panel

**Goal:** one short list of who's here now, at the top of the chat. No "was here".

Why: each browser has its own cookie, so one person on three browsers is three users. "Was here" kept
all of them forever and filled the room page.

- Drop "was here" everywhere. Presence keeps only "watching now": an open socket, or the 15 s
  reconnect grace. A clean close still leaves at once. Someone gone past the grace is forgotten.
- Migration `0010`: drop `room_visitors`. Remove `Rooms.Visit`, `Rooms.Visitors` and
  `Presence.Load`, and `wasHere` from the `presence` message (Go, TS and `protocol.json` together).
  Remove the `wasHere` string.
- Room page: the list under the player goes. The watching-now list moves to the top of the chat panel,
  under its header, like a chat app's "online" list. The same panel in the side panel, the bottom sheet
  and fullscreen. The homepage cards stay as they are.
- Work the look with the `frontend-design` skill. In plan mode, ASCII wireframes of the chat panel with
  the list: wide screen, portrait phone bottom sheet, and 1, 4 and 12 people with long names. The list
  must never push the messages off a phone screen: cap its height or let it scroll sideways. Tokens and
  shared classes from `web/src/app.css` only.
- Update AGENTS.md: the presence rule under "Rooms", and chat's UI under "Chat".

**Done when (tests):**
- Presence, table-driven: gone past the grace → not in the list; clean close → out at once; two tabs →
  one entry; a drop under 15 s → still in.
- Migration test: `room_visitors` is gone, `rooms` and `messages` rows survive.
- Protocol fixture decodes the same in Go and TS without `wasHere`.
- The rest pass unchanged.

**By hand:** one room, three browsers with different names, then close two. The list shrinks, and
nothing stays behind. Screenshots of the chat panel at 360×800, phone landscape and desktop, with one
and many people.

**Not here:** merging one person's browsers into one user (needs accounts, out of scope). A count or
list while the chat is closed.

### [x] 20. Field test

**Goal:** real devices, real network.

- Over Tailscale: two PCs, an iPhone, an Android phone, a tablet.
- A full episode together. Lock a phone. Switch Wi-Fi. Restart the host mid-movie.
- Write down every problem as a new slice or a fix.

### [ ] 21. A name is a person

**Goal:** two browsers with the same name are one person: one color in chat, one entry in "watching
now", the same "own" messages.

Why: each browser has its own cookie, so one person on a phone and a laptop is two users with two
colors. There are no accounts, so the name is the only thing that can join them.

- Names match ignoring case: `ali` and `Ali` are one person. The match key is the name in NFC, then
  case-folded with `golang.org/x/text` (`cases.Fold`). Turkish isn't special: `ALİ` and `ali` stay
  two people. SQLite's `lower()` and `NOCASE` fold only ASCII, so the key is made in Go and registered
  as a SQLite function (`sqlite.RegisterDeterministicScalarFunction`), so the migration can use it too.
- Migration `0012`: `users` becomes one row per person (`id`, `name`, a unique `name_key`). A new
  `tokens` table maps each cookie's token hash to a user. Users that share a key merge into the oldest:
  its name spelling stays, every token and message moves to it, and the others go. Messages keep
  pointing at `users (id)`, so nothing else changes shape.
- Picking a name that exists makes this browser that person. A new name makes a new person.
- Rename moves only this browser: it points at the new name, or a new person. The old person keeps its
  name, its messages and its other browsers. Renaming to your own name in another case (`mom` → `Mom`)
  changes the spelling for everyone: it's the only way to fix one.
- A person with no browser left stays: their messages still show their name.
- Presence, colors and "delete own message" already go by user id, so they follow with no change. No
  socket message changes.
- Update AGENTS.md: under "Users", duplicates are no longer allowed: a name is a person, and anyone who
  types it becomes them (the tailnet is trusted, so they can delete that person's messages too). The
  slice 19 note about merging browsers needing accounts goes.

**Done when (tests):**
- Name key, table-driven: `Ali`/`ali`/`ALI` one key, `ALİ`/`ali` two, NFC and NFD `ş` one key, spaces
  trimmed.
- Two browsers pick `Ali` and `ali` → one user, one presence entry; each can delete the other's message.
- Rename to a new name → this browser only; the other browser and the old messages keep the old name.
  Rename to a taken name → joins that person. `mom` → `Mom` → spelling changes for both browsers.
- Migration test: three users `Ali`, `ali`, `Can` with messages → two users, every message and token
  kept, `Ali`'s spelling wins. `foreign_key_check` clean.
- The rest pass unchanged.

**By hand:** two browsers, both pick `Ali` (one types `ali`), chat in one room: one color, one entry in
"watching now". Rename one to `Bo`: two people again, the old messages still say `Ali`.

**Not here:** passwords or any other proof of who you are. Choosing your own color.
