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
