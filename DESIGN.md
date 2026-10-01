---
name: PauseTogether
description: Distance can't pause us.
colors:
  midnight: "#16122b"
  dusk: "#231d40"
  haze: "#a69dcb"
  moonlight: "#eee9ff"
  lamp: "#ffc15e"
  ember: "#ff7a6b"
  line: "color-mix(in oklab, #a69dcb 28%, transparent)"
  black: "#000"
  white: "#fff"
typography:
  display:
    fontFamily: "Big Shoulders Variable, Arial Narrow, sans-serif"
    fontSize: "3.5rem"
    fontWeight: 800
    lineHeight: 0.95
  headline:
    fontFamily: "Big Shoulders Variable, Arial Narrow, sans-serif"
    fontSize: "2.5rem"
    fontWeight: 700
    lineHeight: 1
  title:
    fontFamily: "Big Shoulders Variable, Arial Narrow, sans-serif"
    fontSize: "1.75rem"
    fontWeight: 700
    lineHeight: 1.05
  title-small:
    fontFamily: "Big Shoulders Variable, Arial Narrow, sans-serif"
    fontSize: "1.25rem"
    fontWeight: 700
    lineHeight: 1.3
  body:
    fontFamily: "Atkinson Hyperlegible Next Variable, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.5
  body-large:
    fontFamily: "Atkinson Hyperlegible Next Variable, system-ui, sans-serif"
    fontSize: "1.25rem"
    fontWeight: 400
    lineHeight: 1.3
  label:
    fontFamily: "Atkinson Hyperlegible Next Variable, system-ui, sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 400
    lineHeight: 1.4
  button:
    fontFamily: "Atkinson Hyperlegible Next Variable, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: 1.2
rounded:
  control: "0.5rem"
  panel: "1rem"
  full: "9999px"
spacing:
  gutter: "1rem"
  stack: "0.5rem"
  group: "1.5rem"
  section: "2.5rem"
  page-max: "72rem"
components:
  button-primary:
    backgroundColor: "{colors.lamp}"
    textColor: "{colors.midnight}"
    typography: "{typography.button}"
    rounded: "{rounded.control}"
    padding: "0 1rem"
    height: "2.5rem"
  button-primary-disabled:
    backgroundColor: "{colors.dusk}"
    textColor: "{colors.haze}"
  button-quiet:
    textColor: "{colors.moonlight}"
    typography: "{typography.button}"
    rounded: "{rounded.control}"
    padding: "0 1rem"
    height: "2.5rem"
  button-quiet-hover:
    backgroundColor: "{colors.dusk}"
  button-danger:
    backgroundColor: "{colors.ember}"
    textColor: "{colors.midnight}"
    typography: "{typography.button}"
    rounded: "{rounded.control}"
    padding: "0 1rem"
    height: "2.5rem"
  button-small:
    typography: "{typography.label}"
    padding: "0 0.75rem"
    height: "2rem"
  icon-button:
    textColor: "{colors.moonlight}"
    rounded: "{rounded.control}"
    size: "2.75rem"
  field:
    backgroundColor: "{colors.dusk}"
    textColor: "{colors.moonlight}"
    rounded: "{rounded.control}"
    padding: "0.5rem 0.75rem"
    height: "2.5rem"
  row:
    rounded: "{rounded.control}"
    padding: "0.5rem 0.75rem"
    height: "2.75rem"
  transport-main:
    backgroundColor: "{colors.lamp}"
    textColor: "{colors.midnight}"
    rounded: "{rounded.full}"
    size: "3.5rem"
  panel:
    backgroundColor: "{colors.dusk}"
    rounded: "{rounded.panel}"
    padding: "1.25rem"
  pill:
    textColor: "{colors.moonlight}"
    rounded: "{rounded.control}"
    padding: "0.4em 0.75em"
---

# Design System: PauseTogether

## Overview

**Creative North Star: "The Living Room at Night"**

The app is a dark room where the TV is on and one lamp is lit. The walls are deep violet, never pure
black. Text is pale moonlight, quiet text is haze. One warm color, the lamp, marks what matters: the
main action, play, keyboard focus, and "someone is here". Everything else steps back so the film is
the brightest thing on screen.

The interface is quiet and soft-spoken. Surfaces are flat. Depth comes from two violet layers, the
page and the raised surface, plus thin translucent borders. Controls are small in voice but large
under a finger. Titles use a tall condensed face so long film names fit a phone; everything else uses
a face built for easy reading.

Density is low on guest screens: one clear action, plenty of space. The admin page is denser, but it
uses the same few pieces.

**Key Characteristics:**
- Dark only, violet-tinted, with one warm amber accent.
- Flat surfaces: tonal layers and hairline borders, no shadows.
- Condensed display face for titles, legible sans for everything else.
- Every control at least 44 px on touch screens.
- Over the video, controls float on translucent midnight and fade while the film plays.

## Colors

A cool violet night with one warm lamp and one red-orange warning.

### Primary
- **Lamp Amber** (lamp): the one accent. Primary buttons, the play/pause button, focus rings, progress
  bars, the bar beside a room with people in it, your own name in chat. Also the native
  `accent-color`, so checkboxes and the seek bar match. The volume slider is moonlight: it's a setting,
  not progress.

### Secondary
- **Ember Coral** (ember): delete buttons, errors, "Video missing", failed prepare jobs. Never
  decorative.

### Neutral
- **Midnight Violet** (midnight): the page. Also the base for translucent layers over video (55–82%
  mixed with transparent).
- **Dusk Violet** (dusk): raised surfaces: panels, dialogs, fields, menus, the picker, the control
  bar. Also the "off" state of a filled button.
- **Haze Lavender** (haze): quiet text: hints, timestamps, section headings on the homepage,
  archived rooms, placeholders.
- **Moonlight** (moonlight): all main text. Also tinted overlays for hover (7–12% mixed with
  transparent).
- **Line** (line): every border and divider, haze at 28%.
- **Black / White** (black, white): the video box and subtitles only. Subtitle text is white on a
  black 65% backing.

### Named Rules
**The One Lamp Rule.** Lamp Amber is the only warm color. It points at the next action or at people.
Two lamp-colored things fighting on one screen means one of them is wrong.

**The Closed Palette Rule.** Only these tokens exist. Tailwind's own palette is cleared in
`web/src/app.css`, so `neutral-700` builds to nothing. Components never use loose hex values.

## Typography

**Display Font:** Big Shoulders Variable (with Arial Narrow, sans-serif)
**Body Font:** Atkinson Hyperlegible Next Variable (with system-ui, sans-serif), upright and italic

**Character:** A tall, condensed marquee face for titles over a calm, highly legible reading face.
Both are bundled with the app, never loaded from a CDN, and both include latin-ext for Turkish.

### Hierarchy
- **Display** (800, 3.5rem, 0.95): the homepage tagline only, from `lg`; Headline size below, so it
  never pushes the rooms off a phone. Balanced wrapping.
- **Headline** (700, 2.5rem, 1): page titles: the room title (Title size on a phone), Admin, "What
  should we call you?", "Room not found".
- **Title** (700, 1.75rem, 1.05): room card titles, homepage and admin section headings, the app
  name in the header.
- **Title Small** (700, 1.25rem, 1.3): chat panel header, picker header and group headings.
- **Body** (400, 1rem, 1.5): everything people read: chat messages, notes, video names.
- **Body Large** (400, 1.25rem, 1.3): the switch confirm ("You're at 1:40:00. Switch to …?"), "Tap to join".
- **Label** (400, 0.8125rem, 1.4): meta lines, watching lists, the control bar, chat timestamps and
  quotes, small buttons. Times use tabular numbers.
- **Button** (600, 1rem, 1.2): button text.

### Named Rules
**The Six Sizes Rule.** Only `text-sm` through `text-3xl` exist. `text-xs` builds to nothing. Nothing
on screen is smaller than 0.8125rem.

**The Marquee Rule.** Big Shoulders is for titles and headings only. Never body text, buttons or
labels.

## Layout

- Pages sit in a centered column, at most 72rem wide, with a 1rem side gutter.
- The homepage is two columns from `lg` (tagline and "Watch something" on the left, sticky; rooms on
  the right, 2:3), and one column below.
- The room page goes full width. Wide landscape screens put the chat beside the video (18rem, 20rem
  from `lg`). Portrait screens put the chat under the video, filling the rest of the screen.
- On phones the video runs edge to edge (it cancels the gutter). On a tall window the video box stops
  growing where the control bar still fits, between black bars.
- Fullscreen fills the player wrapper, not the `<video>`, so chat and subtitles stay on top. Where
  real fullscreen isn't available, the wrapper fills the window with CSS.
- Spacing steps: 0.5rem inside lists and stacks, 1.5rem between groups, 2.5rem between sections,
  4rem between homepage columns on wide screens.
- Text over video (subtitles, pills) sizes from the video box width (container query units), so it
  grows in fullscreen.

## Elevation & Depth

The system is flat. There are no shadows. Depth comes from tone: midnight is the floor, dusk is
anything raised, and a thin line border separates neighbors. Over the video, layers are translucent
midnight (55–90%), with a 6px backdrop blur on the round transport buttons only.

### Named Rules
**The Two Floors Rule.** A surface is either the page (midnight) or raised (dusk). A third level gets
a border or a translucent tint, not a new color.

## Shapes

Two radii carry the whole system. Controls (buttons, fields, rows, menus, pills) use gentle corners
(0.5rem). Containers (the player, dialogs, the picker, the folder picker) use softer ones (1rem).
Fully round shapes are kept for the transport buttons, "Tap to join", progress bars, presence dots,
and the lamp bar on a room card. Borders are always 1px in line color; a 2px left border marks a quote
(line color) or the message you're replying to (lamp).

## Components

### Buttons
Quiet and soft-spoken: flat fills, short labels, a short 150ms color change on hover.
- **Shape:** gentle corners (0.5rem), 2.5rem tall, 2.75rem on touch screens.
- **Primary:** lamp fill, midnight text. Hover mixes 18% white into the lamp. One per screen region.
- **Quiet:** transparent with a line border, moonlight text. Hover raises it to dusk with a haze
  border. The default for everything that isn't the main action.
- **Danger:** ember fill, midnight text. Only inside a confirm step.
- **Small:** 2rem tall, label size. Still 2.75rem on touch screens.
- **Disabled:** quiet buttons fade to 45%. Filled buttons don't dim to brown: they turn dusk with haze
  text and a line edge (inset, so the size holds), like a lamp switched off. A form's Save stays off
  until something in it changes, so a settings page at rest shows no lit lamps.
- **Focus:** a 2px lamp outline, 2px offset, on every focusable element.

### Icon Buttons
A 2.75rem square holding one inline SVG icon, moonlight. Hover adds a 10% moonlight tint. A toggle
that is on (its panel is open) keeps a 12% tint. The subtitles button is haze while subtitles are off
and moonlight while on: state, not the lamp.

### Inputs / Fields
- **Style:** dusk fill, a border of haze at 60% (stronger than line: the edge is what shows where to
  type, 3:1 on midnight and dusk), gentle corners, 2.5rem tall (2.75rem on touch).
- **Focus:** the border turns lamp, plus the lamp focus ring.
- **Disabled:** 45% opacity. Placeholders are haze at full opacity.

### Rows
A list item that is one big button: picker videos, shows, folders, menu items. Full width, 2.75rem
tall, transparent until hover (7% moonlight tint). Greyed to 45% when the device can't play it.

### Room Cards
Not boxed. A room is a list row with a title-size condensed name that underlines in lamp on hover,
then a haze line: the video name below a custom name, or the version label ("1080p.BrRip.x264") below
a video title. Release tags never sit in the condensed face. Then one label-size line: where the
room is in haze ("1:02:13 of 2:34:27", "Not started", "Finished"), or "Video missing" in ember when it
can't play, followed by "watching" with a lamp dot.
When someone is in the room, a thin lamp bar lights its left edge: the lamp is on. Rooms with people
in them come first, and the first one gets the screen's lamp: a primary "Join" ("Watch something"
turns quiet). A room with nobody in it has no Join button, so a haze chevron at the row's end says the
row opens it. Archive and Delete hide behind a quiet "Manage" toggle beside the Rooms heading.
Archived rooms show their title in haze.

### Player (signature component)
- **Video box:** black, softer corners (1rem) on wide screens, edge to edge on phones.
- **Transport:** three round buttons over the middle of the video: back 10 s, play or pause, forward
  10 s. The side ones are translucent midnight with a 6px blur; play/pause is the lamp. They shrink to
  92% while pressed. They fade after 3 s of no input while playing.
- **Control bar:** dusk, a line border on top, label size. In fullscreen it lies over the foot of the
  video on a black-to-transparent fade (80% black at the bottom), and fades out with the transport
  (300ms). Subtitles slide up while it shows.
- **Holds the middle:** "Tap to join" (a large round primary button), "Preparing… 42%" (a pill with a
  thin lamp progress bar on dusk), "Waiting for …", "Video missing", "Can't play", "Couldn't get this video ready", "Getting the room
  ready…" (a pill, after a moment), and the end. The waiting panel and the three problems are dusk
  panels at 90%, compact on a phone's small video box.
  - "Waiting for Alice" in body large, then why and for how long in haze label ("stepped away · 0:12"),
    then a quiet button that names its effect ("Play without Alice"). The person the room waits for
    reads "Everyone's waiting for you" and "Don't wait for me".
  - The problems share one panel: a body-large headline, a line saying what's left to do, and a lamp
    "Pick another video" when another pick helps. "Video missing" (the pick resumes where the room
    was) and "Couldn't get this video ready" are ember. "This device can't play HEVC" is moonlight:
    it's a limit, not an error. It says to try another device, that the room won't wait for this one,
    and that chat still works. No pick there: the others can still watch.
  - The end: a TV episode offers a lamp "Next episode". Anything else gets a title card, "The end" in
    the display face (3xl, 2xl on a phone's box), over a lamp "Watch something else".
- **Subtitle panel:** a dusk strip over the bar, in two labelled groups: "For everyone" (subtitle,
  timing) and "On this screen" (size). Group labels are semibold moonlight, field labels haze.

### Pills
A short line over the video: "Host is offline, reconnecting…" (with a small ember signal icon), "Alice
paused" (with a small lamp pause icon), "Alice is 3 s behind", chat toasts. Midnight at 82%, gentle corners, sized with the video width (at least 1rem), so they stay
readable in fullscreen.

### Chat
A panel with a title-small header, then the "watching now" list in label size (at most two lines),
then messages. Messages are not bubbles: each is a row with the name in bold (yours in lamp), the
video time in haze, then the text. Replies quote with a 2px line border on the left. The reply being
written shows a lamp left border instead.

### Subtitles
White text on a black 65% backing with small corners, sized from the video width. Only `<i>` and
`<b>` styling survives.

### Navigation
A plain header row: the lamp pause icon and the app name on the left, your name as a quiet text
button with a chevron on the right. Its menu is a dusk panel with a line border, holding rows.

The room page's title sits with the video's name right under it, and its actions beside it: a quiet
"Switch video", and a "⋯" menu (the same panel) with "Next episode" and "Rename room". Nothing else
stands between the title and the video, so the video starts high on a phone.

## Do's and Don'ts

### Do:
- **Do** use only the tokens in `web/src/app.css`: six named colors, line, black and white.
- **Do** keep Lamp Amber for the next action, focus, progress and presence.
- **Do** make every control at least 44 px (2.75rem) on touch screens.
- **Do** separate surfaces with the midnight/dusk step and a 1px line border.
- **Do** put text over video on translucent midnight and size it from the video box width.
- **Do** leave room for longer Turkish strings: wrap and `break-words` instead of fixed widths.
- **Do** honor reduced motion: transitions turn off and animations collapse to near zero.

### Don't:
- **Don't** use Tailwind's built-in colors or sizes, or loose hex values in components.
- **Don't** use `text-xs` or any size below 0.8125rem.
- **Don't** set body text, buttons or labels in Big Shoulders.
- **Don't** load fonts or anything else from a CDN.
- **Don't** use ember for anything but delete and errors.
