# Chat In-Flow Streamlining — Design

**Date:** 2026-06-29
**Status:** Approved (pending spec review)

## Problem

The chat UI feels heavy because of *in-flow choices* — moments during a
conversation where the user is forced to pick between near-identical options or
fight clunky controls. Through brainstorming, the pain narrowed to two root
causes and three concrete areas:

**Root causes**
1. **Too many overlapping options** — branch / fork / regenerate / switch-fork
   are four buttons for nearly the same idea. The user *never uses branching*,
   so these are pure clutter that crowd the message UI and invite accidental taps.
2. **Mechanically clunky selection** — controls require hover + precise clicking;
   the user is frequently on **mobile**, where this breaks down entirely.

**Headline pain:** typing slash commands like `/clear` on a mobile keyboard is
*extremely* annoying. This is the single biggest friction.

**Out of scope (explicitly):** consolidating branch/fork/regenerate into a
unified "variants" model. The user does not use branching at all, so the right
move is to *remove* it from the UI, not redesign it. The Plan/Act toggle and the
model picker are left as-is.

## Goals

- Trigger common actions **without typing** — mobile-first.
- **Declutter** message actions by removing unused branching controls.
- Make in-response choices **keyboard-operable** (desktop nice-to-have).

## Non-Goals

- No backend data-model changes for branching. Branch/fork HTTP endpoints stay;
  only their **frontend entry points** are removed.
- No change to Plan/Act mode, model picker, project picker, or persona flow.
- No new slash commands; we only change *how* existing ones are reached.

---

## Section A — Command access without typing (headline)

Today the only way to run a command is to type `/` in the composer, which opens
a command menu (`ChatInput.tsx` ~264-270; 9 commands: `/new`, `/status`,
`/model`, `/export`, `/search`, `/find`, `/clear`, `/compact`, `/reset`).

Two additions, both reusing the **existing command handlers** (no new command
logic — only new triggers that dispatch the same actions):

### A1. Quick-action chips (always visible)
Three tappable chips for the most common actions, rendered in the composer
toolbar row (alongside the existing upload/voice/plan-act buttons in
`ChatInput.tsx`):

| Chip | Command | Action |
|------|---------|--------|
| **Nový chat** | `/new` | Start a new thread |
| **Hledat** | `/search` (fall back to `/find`) | Open search |
| **Smazat historii** | `/clear` | Clear thread messages (soft-delete) |

- One tap = the action fires immediately. `Smazat historii` keeps its existing
  confirmation (clear is destructive, even if soft-delete is recoverable).
- **Keyboard shortcut for new chat:** `Ctrl/Cmd+Shift+O` triggers the same
  `/new` action as the chip. Chosen to match the common "new chat" convention and
  to avoid collision with the existing chat shortcuts (`Shift+Tab` = Plan/Act,
  `Cmd/Ctrl+F` = search). Registered globally for the chat view, with
  `preventDefault` so it doesn't fall through to the browser.
- Chips are icon + short label; on narrow viewports they collapse to icon-only
  to fit, but remain a single tap (no hover).
- The model picker is intentionally **not** a chip — it stays in the thread
  settings panel / header badge as today.

### A2. Command-menu button (the rest)
A persistent button in the composer toolbar (a `/` or list/`☰` glyph) that opens
the **same command menu** that typing `/` produces — but via tap, no keyboard.

- Opens an overlay list of all commands with their human labels/descriptions,
  filtered to what's relevant (e.g. hide `/find` when already covered, hide
  commands that don't apply to the current thread state).
- Tapping an item runs it and closes the menu.
- Typing `/` in the input still works unchanged for desktop power users.

### Behavior / interface
- New trigger surface, **same dispatch**: chips and the menu button call the
  identical command handlers the typed `/command` path already invokes. There is
  one source of truth for "what `/clear` does"; we only add callers.
- Mobile-first: all targets ≥ 44px touch size, no hover dependency.

---

## Section B — Declutter message actions

`MessageActions.tsx` currently renders up to six controls per message bubble:
Copy (40-56), Edit (58-69), **Branch (71-82)**, Regenerate (84-95),
**Fork (97-106)**, Hide/Unhide (108-117). On mobile these are hover-revealed,
so on touch they are effectively unreachable or always-on clutter.

Changes:
1. **Remove Branch and Fork** entry points from the message action row entirely.
   - Delete the two buttons and their handlers/wiring in `MessageActions.tsx`,
     and the now-orphaned UI they open: the fork-point branch switcher in
     `ChatView.tsx` and `ForkThreadModal.tsx` usage, plus the
     `ThreadForkBadges` fork popover in the header if it becomes dead.
   - **Backend endpoints (`/threads/:id/branch`, `/threads/:id/fork`,
     `/active-branch`) remain** — only the frontend triggers are removed. No
     migrations, no model changes.
2. **Keep** Copy, Edit (user messages), Regenerate (last assistant message),
   Hide/Unhide. These are the actions the user actually wants.
3. **Mobile reachability:** message actions must be reachable by **tap** on
   touch devices, not hover only — reveal the action row on tap of the bubble
   (or show a compact "…" affordance). Desktop hover behavior is preserved.

### Dead-code note
Removing the branch/fork UI may orphan `ForkThreadModal.tsx`,
`ThreadForkBadges.tsx`, branch-switch handlers (`handleSwitchBranch`), and the
client API methods for fork/branch/active-branch. These are deleted if fully
unused after the change; if any (e.g. parent-thread link badge) is still
referenced, it is left intact. The two **pre-existing API bugs** found during
exploration (branch POST field `parent_message_id` vs client `parent_id`;
switch route `/active-branch` vs client `/branch`) become moot once the UI is
removed — no fix needed since the callers are deleted.

---

## Section C — Keyboard-operable in-response choices (nice-to-have)

When Claude's reply contains choices, let the user answer from the keyboard.

1. **Numbered lists** (`SelectableOptions.tsx`): already supports arrow-key
   navigation + Enter. Add **direct number selection** — pressing `1`–`9` picks
   that option immediately. Keep arrow + Enter.
2. **AskUser panel** (`AskUserPanel.tsx`, tool-call prompts): add the same
   keyboard model — press the option's **number** to toggle/select, **Enter** to
   submit. Free-text questions keep their textarea; Enter submits, Shift+Enter
   newline. Make its visual + interaction model consistent with
   `SelectableOptions`.

This section is the lowest priority and can ship separately from A and B.

---

## Approach trade-offs considered

- **Consolidate branching into "variants"** (rejected): elegant, but the user
  doesn't branch at all — building a unified variant navigator would add code for
  an unused workflow. Removal beats consolidation here.
- **Command palette only (no chips)** vs **chips only (no menu)** (rejected in
  favor of both): the user explicitly wanted persistent chips for the top
  actions *and* a menu for the long tail. Both, sharing one dispatch layer.

## Testing

- **Frontend (Vitest):**
  - Chips dispatch the correct command handler on click/tap (mock the handler,
    assert it's called) — one test per chip.
  - Command-menu button opens the menu and selecting an item dispatches the same
    handler as typing the command.
  - `Ctrl/Cmd+Shift+O` dispatches the same `/new` handler as the chip.
  - `MessageActions` no longer renders Branch/Fork buttons; still renders Copy /
    Edit / Regenerate / Hide under the right conditions.
  - `SelectableOptions`: pressing a number key selects the matching option.
  - `AskUserPanel`: number key selects, Enter submits.
- **Manual / mobile:** verify on a narrow viewport (DevTools device mode or the
  real PWA) that chips and message actions are reachable by tap with no hover.
- **Go:** no backend changes expected; `make check` must still pass (ensures no
  broken imports from deleted client methods).

## Rollout

Sections are independent and can land in order **A → B → C**, each behind its own
commit and each leaving `make check` green. A is the highest-value, ship-first
piece.
