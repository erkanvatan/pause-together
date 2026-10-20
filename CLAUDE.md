# CLAUDE.md

The project spec and rules are in @AGENTS.md

## Claude specific

- No attribution lines in commits or PRs: no `Claude-Session` link, no "Generated with Claude".
- Checking the player at phone size in Chrome (the window can't resize): open
  `http://localhost:5173/`, then replace the page with a same-origin
  `<iframe src="/rooms/{id}" style="width:412px;height:893px">` and drive it through
  `iframe.contentDocument`. The automation tab counts as hidden: no video frames, no
  `requestAnimationFrame`, and `bind:clientHeight` (ResizeObserver) only updates when a screenshot
  forces a frame, so take one before measuring. Vite reloads the page after each edit, which wipes
  the iframe: build it again.
- Checking on a real phone with BrowserStack: the BrowserStack MCP refuses `localhost`, so start a Live
  session on any public URL, then open its dashboard link in Chrome with
  `url=http%3A%2F%2Flocalhost%3A5173%2F…` and `local=true`, and drive it there. Type `http://`, not
  https. The phone may open google.com anyway: type the address in its own address bar then. A typed
  `:` arrives as `;` (on Android at least): tap `!#1`, then the phone keyboard's own `:`. The user's `BrowserStackLocal` tunnel must run on the account logged in to Chrome. BrowserStack
  drops some taps and typed keys; typed text that misses a field hits the player's keys (F, M, Space).
  Tap a bar button within a second of waking the controls: they fade after 3 s, and a tap on faded
  controls only wakes them. To see what the phone does, a throwaway Vite `configureServer` middleware
  that logs `/__log` requests, fed by `fetch('/__log?…')` from the page, works.
