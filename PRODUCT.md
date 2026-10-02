# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

- **Guests:** family or a partner who live far away, in another city or country. A small group.
  Some speak Turkish; a Turkish UI is planned. They join from a PC, tablet or phone. They want to
  press play and watch, not set anything up.
- **The host:** one person whose machine holds the video library. They manage libraries, files that
  don't play, the job queue and the cache from a local-only admin page.
- **Self-hosters:** the project is public. Other people set it up for their own family from the README.
  The admin page and setup must make sense to someone who has never seen the code.

## Product Purpose

Watch the same video at the same second with people far away, from your own library. Success: a
movie night where nobody counts down "3, 2, 1, play", nobody falls behind, and nobody fiddles with
settings mid-film.

## Positioning

- **The room waits for everyone.** Someone buffering or stepping away pauses the room for all. Sync
  is the server's job, not the viewers'.
- **Your own files, original quality.** Video is never re-encoded. No streaming service, no account,
  no upload to anyone's cloud.
- **Private by network.** Guests reach it over Tailscale. No sign-up; a guest picks a name and is in.

## Operating Context

- Guests open one link from a message, pick a name once, tap "Tap to join", and watch.
- A room is one shared video plus its chat. Rooms last until the host deletes them, so a show can be
  watched over many evenings in the same room.
- Chat sits beside or under the video and works in fullscreen. It's for talking during the film.
- Guests may have no route to the internet beyond the tailnet: nothing can load from a CDN.
- Pages are plain HTTP, so HTTPS-only browser features (clipboard, Wake Lock) are not available.
- The host's upload speed limits quality: every guest streams the original file.

## Capabilities and Constraints

The full, binding spec is `AGENTS.md`. Facts that shape design work:

- Roles: host (admin port, `localhost:8421`) and guests. No accounts. Everyone can create rooms,
  control playback, switch videos, chat, archive rooms.
- Library from local folders with Plex naming: Movies, TV Shows, Other Videos. Names only: no
  artwork, no posters, no online metadata. Any design must work with text titles alone.
- One `<video>` per room page, inline on iPhone, our own controls, subtitles in our own overlay.
- Touch controls at least 44 px.
- All UI text lives in `web/src/lib/strings.ts`, English now, Turkish next. Layouts must survive
  longer Turkish words.

## Brand Commitments

- Name: **PauseTogether**. Tagline: *"Distance can't pause us."* Line: *"Same show. Same second.
  Different places."*
- Voice: short, plain, warm sentences. Say what happened and what to do ("Can't reach the server.
  Retrying…"). No jargon in guest-facing text.

## Evidence on Hand

- `web/static/favicon.svg` is the only brand asset. No logo files, screenshots, users' quotes or
  usage numbers exist. Don't invent them.

## Product Principles

1. **Watch together first.** When sync, comfort and features pull apart, the shared moment wins.
2. **Nothing to learn for guests.** A non-technical relative gets from link to watching without help.
3. **The film is the star.** Controls, chat and notes step back while the video plays.
4. **Honest about limits.** Say plainly when a device can't play a file or the host is offline. Never
   a silent black screen.
5. **Self-hostable by strangers.** Host-facing screens explain themselves without the source code.

## Accessibility & Inclusion

- Two languages: English now, Turkish soon. No text baked into images; leave room for longer strings.
- Touch screens: controls at least 44 px.
