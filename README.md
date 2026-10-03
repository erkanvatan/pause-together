<div align="center">

# PauseTogether

*Distance can't pause us.*

A self-hosted watch-together app. Same show. Same second. Different places.

[Features](#features) • [How it works](#how-it-works) • [Getting started](#getting-started) • [Tailscale](#connecting-devices-with-tailscale) • [Development](#development)

</div>

One machine serves its own video library. Friends reach it over [Tailscale](https://tailscale.com) and
watch in sync from PCs, tablets and phones. When one of them buffers or steps away, the room waits.

## Features

- **Your library, Plex-style.** Movies, TV shows and other videos, read from local folders with Plex
  naming. New files are picked up on their own.
- **Original quality.** Video is never re-encoded. Each video is prepared once into a plain MP4, so
  seeking just works.
- **Real sync.** The server owns the room's state. Every player follows it, with small speed nudges
  for small drift.
- **The room waits for everyone.** Someone buffering, or away for more than 3 s, pauses the room.
  Anyone can press "Play anyway".
- **Subtitles.** Embedded and sidecar text subtitles, converted to WebVTT, with a shared time offset.
  Old Turkish, Western and Cyrillic `.srt` files decode correctly.
- **Join, don't split.** Pick a video that already has a room, and you're offered that room first.
- **Binge-friendly.** "Next episode" switches at once and keeps the audio and subtitle language. No
  autoplay: the end of an episode waits for someone to press it.
- **Chat.** One chat per room, with replies, video timestamps and a color per person. It stays
  visible in fullscreen.
- **No accounts.** Guests pick a name. The same name on a phone and a TV is the same person. The host
  manages everything else from a local-only admin page.
- **Works on phones.** Plays inline on iPhone, with its own controls. In fullscreen, the phone turns
  upright with the chat under the video, and sideways without it.
- **Looks after itself.** Starts at boot, takes a daily database backup, and deletes prepared copies
  nobody has watched for a while.

## How it works

```mermaid
flowchart LR
  subgraph Host machine
    M[(Media folder<br/>read-only)] --> A[PauseTogether<br/>Go + SQLite + ffmpeg]
    A --> C[(Data dir<br/>db, cache, backups)]
    H[Host browser] -- "localhost:8421<br/>admin" --> A
  end
  G1[Guest PC] -- "Tailscale :8420" --> A
  G2[Guest phone] -- "Tailscale :8420" --> A
```

- One Go binary with the SvelteKit app built in, running in Docker.
- Two ports. The **guest port** (`8420`) serves the app. The **admin port** (`8421`) adds the admin
  API and only listens on `127.0.0.1`.
- Tailscale is the only access control. Anyone who reaches the guest port is trusted.

> [!IMPORTANT]
> Every viewer streams the original file. The host's upload speed must cover the video's bitrate
> times the number of guests.

## Getting started

### Prerequisites

- Linux host with [Docker](https://docs.docker.com/engine/install/) and Docker Compose
- [Task](https://taskfile.dev/installation/)
- [Tailscale](https://tailscale.com/download) on the host and on every guest device

### Setup

1. Clone the repo and copy the settings template:

   ```sh
   git clone https://github.com/erkanvatan/pause-together.git
   cd pause-together
   cp .env.example .env
   ```

2. Edit `.env`:

   | Variable | What it is | Default |
   | --- | --- | --- |
   | `MEDIA_ROOT` | Folder with all your media. Mounted read-only. Must exist. | `~/Videos` |
   | `DATA_DIR` | Folder for the database, cache and backups. `task up` creates it. | `~/.local/share/pausetogether` |
   | `PUBLIC_BIND` | Host IP for the guest port. Set it to your [Tailscale IP](#on-the-host). | `127.0.0.1` |
   | `GUEST_PORT`, `ADMIN_PORT` | Host ports. | `8420`, `8421` |

> [!CAUTION]
> Never set `PUBLIC_BIND` to `0.0.0.0`. Docker bypasses `ufw`, so that opens the app on every
> network the host joins, café Wi-Fi included.

3. Let Docker bind the Tailscale IP even when it starts before Tailscale does:

   ```sh
   echo 'net.ipv4.ip_nonlocal_bind=1' | sudo tee /etc/sysctl.d/99-pausetogether.conf
   sudo sysctl --system
   ```

4. Start Docker at boot, so PauseTogether comes back by itself after a restart:

   ```sh
   sudo systemctl enable docker.service containerd.service
   ```

5. Build and start:

   ```sh
   task up
   ```

   It keeps running across reboots until you stop it with `task down`. After that, it stays off
   until the next `task up`.

6. Open `http://localhost:8421` on the host. Go to the admin page and add a library: a folder under
   your media root, plus its type.

7. Let guests in: see [Connecting devices with Tailscale](#connecting-devices-with-tailscale).

> [!TIP]
> `task logs` follows the server log. `task down` stops the server, so check nobody is mid-movie.

### Connecting devices with Tailscale

Guests reach the host through [Tailscale](https://tailscale.com). It's a free app that links your
devices into one private network, even across different homes. No router setup, no open ports. Each
device gets a fixed address starting with `100.`.

You set it up once on the host. Each guest installs the app, accepts your invite and opens one link.

#### On the host

1. Install Tailscale and log in:

   ```sh
   curl -fsSL https://tailscale.com/install.sh | sh
   sudo tailscale up
   ```

   `tailscale up` prints a link. Open it and sign in with a Google, Microsoft, GitHub or Apple
   account. That creates your free Tailscale account.

2. Get the host's Tailscale IP:

   ```sh
   tailscale ip -4        # e.g. 100.101.102.103
   ```

   Put it in `.env` as `PUBLIC_BIND=100.101.102.103`, then run `task up`. If the server is already
   running, run `task up` again to pick up the change.

3. Turn off key expiry for the host. By default Tailscale logs every device out after 180 days, and
   guests would find the server gone. On the [Machines page](https://login.tailscale.com/admin/machines),
   open the host's `⋯` menu and pick **Disable key expiry**.

4. Share the host with each guest. On the same page, open the host's `⋯` menu and pick **Share…**.
   Send the guest the link, or enter their email. A share gives them this one machine, not your other
   devices.

#### On each guest device

1. Open the share link from the host. Sign in with a Google, Microsoft, GitHub or Apple account, and
   accept the shared machine. This is done once per person, not per device.
2. Install Tailscale from the [download page](https://tailscale.com/download), or the App Store or
   Google Play on phones and tablets. Sign in with the same account and turn it on. Phones ask to add a
   VPN: allow it.
3. Open the host's address in a browser, with `http://` and the port: `http://100.101.102.103:8420`.
   Pick a name, then bookmark the page.

Give every guest the same address, the Tailscale IP. The admin page shows it as the guest link, and
each room shows its own link to copy into a message. A browser remembers its name per address
(`localhost`, the IP, the MagicDNS name). A guest who switches address has to type their name again.
The same name makes them the same person again.

> [!NOTE]
> Tailscale must be on while watching. On phones it's a switch in the app, and it can turn itself off.
> If the page won't load, check that first.

#### If something doesn't work

| Problem | What to check |
| --- | --- |
| Page won't load | Tailscale is on, on both the host and the guest device. `PUBLIC_BIND` is the host's Tailscale IP, and `task up` ran after you set it. The address has `http://` and `:8420`. |
| Worked before, stopped | Tailscale got turned off on the device. Or the host's key expired: see step 3 above. |
| Video keeps buffering | While the guest watches, run `tailscale status` on the host and find their device. `direct` is good. `relay` means traffic goes through Tailscale's servers, which is slower. Strict networks (work, school, some mobile data) often force a relay. Also check the host's upload speed. |

### Naming your files

PauseTogether reads everything from folder and file names. It never looks anything up online, so the
names have to be right. Movies and TV shows follow [Plex's naming rules](https://support.plex.tv/articles/naming-and-organizing-your-movie-media-files/).
A file that doesn't match is skipped and shows up on the admin page with the reason.

#### Libraries

A library is one folder inside `MEDIA_ROOT` with one type: **Movies**, **TV Shows** or
**Other Videos**. Keep each type in its own folder:

```text
~/Videos/            ← MEDIA_ROOT
├── Movies/          ← library, type Movies
├── TV/              ← library, type TV Shows
└── Home Videos/     ← library, type Other Videos
```

Libraries can't overlap. You can't add `~/Videos` as one library and `~/Videos/Movies` as another.

#### Movies

The file name needs the title and the year in parentheses. The folder is optional, and only the file
name counts.

```text
Movies/
├── Dune (2021)/
│   ├── Dune (2021).mkv
│   ├── Dune (2021).en.srt
│   └── Trailers/                          ← extras: skipped quietly
├── Heat (1995).mkv                        ← no folder: fine
└── Alien Collection/                      ← collection folder: fine
    └── Alien (1979)/Alien (1979).mkv
```

- **Editions:** `Blade Runner (1982) {edition-Final Cut}.mkv`. Two editions can share a folder.
- **Versions:** text after the year is a version label: `Dune (2021) - 4K.mkv`.
- **Other `{...}` tags** are ignored, like `{imdb-tt1160419}`.

#### TV shows

Every episode sits in a show folder. The show's name comes from that folder, not from the file. The
season and episode come from the `s01e02` in the file name.

```text
TV/
└── Severance (2022)/                      ← show folder: the show's name
    ├── Season 01/
    │   ├── Severance (2022) - s01e01 - Good News About Hell.mkv
    │   └── Severance (2022) - s01e02 - Half Loop.mkv
    ├── Season 2/                          ← no leading zero: fine
    │   └── Severance - s02e01.mkv         ← year and episode title are optional
    └── Specials/                          ← or Season 00
        └── Severance - s00e01 - Inside Lumon.mkv
```

- **Season folders are optional.** An episode can sit loose in the show folder.
- **Two episodes in one file:** `s01e01-e02`, `s01e01e02` or `s01e01-02`.
- **Episode titles** come after ` - `. Junk after the episode number (`.1080p.BluRay-GROUP`) is ignored.
- **One level only.** `Show/Season 01/Disc 1/…` is skipped. So is an episode with no show folder.

> [!WARNING]
> Episodes named by date (`Show - 2024-05-01.mkv`) or by absolute number (`Show - 125.mkv`, common for
> anime) are not supported. Rename them to `s01e02` style.

#### Other videos

No rules. The file name is the title, and sub-folders become groups in the picker.

```text
Home Videos/
├── Wedding.mp4                            ← title "Wedding", no group
└── Summer 2024/
    └── Beach Day.mov                      ← title "Beach Day", group "Summer 2024"
```

#### Subtitle files

A subtitle file (`.srt`, `.vtt` or `.ass`) sits next to its video. Its name is the video's name, then
a language code, then optional flags.

```text
Dune (2021).mkv
Dune (2021).en.srt                         ← English
Dune (2021).tur.srt                        ← Turkish: 2- or 3-letter codes both work
Dune (2021).en.forced.srt                  ← forced: only the foreign-language parts
Dune (2021).en.sdh.srt                     ← for the deaf and hard of hearing (.hi works too)
```

- **The language code is required.** `Dune (2021).srt` is skipped.
- **No `Subs/` folder.** Subtitles in a sub-folder aren't read.
- **`hi` means two things.** First it's Hindi (`.hi.srt`). After a language it's the SDH flag
  (`.en.hi.srt`).

Subtitles inside the video file need no naming. They're found on their own.

#### Skipped files

These never show up in the picker:

| Skipped | Examples | On the admin page? |
| --- | --- | --- |
| Extras and samples (Movies and TV only) | `Trailers/`, `Featurettes/`, `Dune (2021)-trailer.mkv`, `sample.mkv` | No |
| Hidden and macOS files | `.hidden.mkv`, `._Dune (2021).mkv` | No |
| Movies split into parts | `Dune (2021) - pt1.mkv`, `cd1`, `disc1` | Yes |
| Anything that breaks the rules above | `Dune.mkv` in a Movies library | Yes |

> [!NOTE]
> Renaming or moving a file makes it a new video, and the old one counts as missing. A room using the
> old one shows "Video missing" once its prepared copy is gone too. Pick the new file there, and
> playback resumes where it was.

### Supported formats

| | Supported |
| --- | --- |
| Containers | `.mkv .mp4 .m4v .mov .avi .webm .ts .m2ts` |
| Video | H.264 (8-bit 4:2:0), HEVC, AV1, VP9. Each device plays what it can decode. |
| Audio | Any. Converted to stereo AAC unless it's already AAC with up to two channels. |
| Subtitles | Text only: SRT, ASS, WebVTT, mov_text. Image subtitles (PGS, VobSub) are not. |

## Development

Everything runs in Docker through Task. You don't need Go or Node on the host.

```sh
task dev        # dev stack with hot reload: http://localhost:5173
task test       # all unit tests (Go + web)
task lint       # golangci-lint, svelte-check
task build      # build the production image
```

Dev uses its own ports and data folder (`./.dev-data`), so it runs safely next to production. It
needs `MEDIA_ROOT` or `DEV_MEDIA_ROOT` set in `.env`. Point `DEV_MEDIA_ROOT` at a scratch folder when
testing file adds and deletes.

**Stack:** Go standard library, SQLite ([modernc.org/sqlite](https://gitlab.com/cznic/sqlite)),
[coder/websocket](https://github.com/coder/websocket), ffmpeg. SvelteKit (Svelte 5, SPA mode) and
Tailwind v4.

The full spec lives in [AGENTS.md](AGENTS.md). The build order lives in [docs/plan.md](docs/plan.md).
