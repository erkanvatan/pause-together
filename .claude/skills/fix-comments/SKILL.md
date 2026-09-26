---
name: fix-comments
description: 'Review code comments and fix them: delete comments that narrate edits ("now uses X instead of Y", "changed to", "previously", "updated"), comments that restate the code or talk to the reviewer, and correct comments whose facts are no longer true (wrong names, numbers, paths, defaults, behaviour). Defaults to the comments touched or affected by the current git diff; accepts a path, file, or git range. Use whenever the user asks to clean up, fix, audit, or check comments or docstrings, mentions stale, outdated, wrong, noisy, or unnecessary comments, or says comments reference old code — even if they only say "the comments are off" after a change.'
allowed-tools: Bash, Read, Edit, Grep, Glob
context: fork
agent: general-purpose
---

# Fix Comments

A comment should describe the code **as it is now**, and explain what the code alone cannot: why it
is this way, what to watch out for. Anything else is noise or, worse, a lie waiting to happen.

Two failure modes are common after an AI edits code:

1. **Change narration.** The comment describes the edit instead of the code: "now uses a map instead
   of a list", "changed to async", "fixed the off-by-one". That is history. It belongs in the commit
   message. Once merged, "now" and "instead of" point at code nobody can see.
2. **Stale facts.** The code changed, a nearby comment did not. It still names an old function, an
   old default, an old count, an old path. Readers trust comments, so a wrong one is worse than none.

## Step 1: Pick the target

- **Argument given** (path, file, glob, or git range like `main..HEAD`): review that.
- **No argument:** review what the working tree changed. Use `git status --porcelain` to list files
  (it respects `status.showUntrackedFiles`, so it stays sane in a bare repo over `$HOME`), and
  `git diff HEAD -- <file>` for the changed lines. Untracked files count as fully changed.

If there is nothing to review, say so and stop.

## Step 2: Find the comments to check

For a diff target, "changed lines" is not enough. A code change makes *untouched* comments stale.
Check:

- Comments on added or modified lines.
- Comments and docstrings in the same function, block, or config section as a change.
- Comments anywhere in the target files that mention a symbol, option, flag, path, or value the diff
  renamed, removed, or changed. Grep the old names from the `-` lines of the diff to find them.

For a path target, check every comment in it.

Comments include docstrings and doc-comments, not only `#` / `//` lines.

## Step 3: Judge each comment

### Delete

- **Change narration** with no lasting reason inside it. Signals: "now", "instead of", "changed",
  "updated", "previously", "used to", "no longer", "new:", "fixed", "moved from", "refactored",
  "replaced", "as before". Signal words are hints, not proof: "retries now and then" is fine.
- **Restates the code.** `# increment counter` above `counter += 1`. `# loop over files` above a loop.
- **Talks to the reader of the diff**, not the reader of the code: "as requested", "per your
  instructions", "I added this to", "Note: this is the fix for".
- **Empty section markers** the file's style does not use, and placeholder comments like
  `# TODO: implement` next to code that is implemented.

### Rewrite

- **Change narration that hides a real reason.** Keep the reason, drop the history.
  `# Now uses flock instead of a pidfile, since pidfiles go stale on crash`
  becomes `# flock, not a pidfile: pidfiles go stale on crash`.
- **Stale fact where the code clearly shows the truth.** Fix the comment to match the code.
  `# Retries 3 times` above `MAX_RETRIES=5` becomes `# Retries 5 times` — or drop the number if the
  constant name already says it.

To catch stale facts, check every concrete claim a comment makes against the code: names (does
that function, variable, file, flag exist — grep it), numbers and counts, defaults, return values,
order of steps, "called from X", "only used by Y", lists ("the three modes") versus what is there.
A claim you cannot verify from the code is not automatically wrong. Leave it.

### Flag, do not touch

- **Comment and code disagree and you cannot tell which is right.** The code may be the bug.
  Changing the comment would hide it. Report it.
- **Commented-out code.** It can be a deliberate toggle (common in config files). Report it.

### Always keep

- Comments that explain why, warn about a trap, or cite an external reason (bug link, spec, quirk).
- Tool directives and machine-read comments: shebangs, `# shellcheck disable=`, `// eslint-disable`,
  `# noqa`, `# type: ignore`, `#pragma`, vim modelines, encoding lines, `# yaml-language-server`.
- License headers, required file headers (for example `@file` / `@brief` / `@depend` blocks),
  section banners and table-of-contents blocks the file already uses.
- `TODO` / `FIXME` that still describe real, open work.

## Step 4: Edit

- Only touch comments. Never change code, even if you spot a bug — report it instead.
- Do not add new comments. This skill removes and corrects; it does not annotate.
- Match the file's comment style (marker, capitalisation, line width).
- When deleting a comment leaves a doubled blank line, remove the extra blank line.

## Step 5: Report

Keep it short. Lead with the count. Use `file:line` so each item is clickable.

```
Fixed 6 comments in 3 files. 1 needs you.

Removed
- scripts/foo:12 — change narration ("now uses rsync instead of cp")
- scripts/foo:40 — restates code

Rewritten
- lib/bar.py:88 — said "3 retries", code does 5

Needs you
- lib/bar.py:120 — comment says "returns None on miss", code raises KeyError. Which is right?
```

If nothing needed fixing, say that in one line.
