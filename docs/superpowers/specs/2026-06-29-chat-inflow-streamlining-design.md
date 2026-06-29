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

| Chip | Command dispatched | Action |
|------|---------|--------|
| **Nový chat** | `/new` | Start a new thread |
| **Hledat** | `/find` | Open in-thread search (the working path; `/search` is a desktop no-op today) |
| **Smazat historii** | `/clear` | Clear thread messages (soft-delete) |

- One tap = the action fires immediately via the existing `onSlashCommand`
  dispatch (one source of truth — chips are just new callers of
  `ChatView.handleSlashCommand`). `Smazat historii` inherits whatever
  confirmation `/clear` has today.
- Chips are icon + short label; on narrow viewports they collapse to icon-only
  to fit, but remain a single tap (no hover).
- The model picker is intentionally **not** a chip — it stays in the thread
  settings panel / header badge as today.

### A2. Command access via the existing CommandPalette

**Discovery during planning:** the codebase already contains a complete, polished
`CommandPalette.tsx` (fuzzy search over commands + recent threads + actions like
New chat / Search / Export / Settings / Toggle theme) and a `useKeyboardShortcuts`
hook — **but neither is wired in anywhere** (orphaned/dead code). This is exactly
the "tap → menu of commands" surface A2 needs, already built. So instead of
building a new menu, we **reuse the palette component**:

- **Wire `CommandPalette` into `ChatPage`** (render it, provide its props from
  existing ChatPage state: `threads`, `selectThread`, `handleNewThread`,
  `setSettingsPanelOpen`, `updateSettings` for theme, in-thread search for
  `onOpenSearch`).
- **Open it without a keyboard:** a visible command button in the chat header
  (mobile + desktop) toggles the palette — this is the mobile entry point, since
  mobile can't press a hotkey. Desktop also gets `Cmd/Ctrl+K`.
- The palette is a self-contained fixed overlay with its own internal
  Arrow/Enter/Escape handling (bound only while open) — **no global key
  conflicts** with ChatView's existing `Shift+Tab` / `Cmd+F`.
- We reuse the palette **component** but **not** the orphaned
  `useKeyboardShortcuts` hook (it depends on a shortcuts-help modal we don't
  have). Instead a small dedicated hook binds only the two hotkeys we need.

### A3. Keyboard shortcut for new chat
A small `usePaletteHotkeys` hook (new, in `hooks/`, independently testable) binds:

- **`Ctrl/Cmd+Shift+O`** → new chat (the requested shortcut; the orphaned hook
  had `Ctrl+Shift+O` but no `Cmd`/meta support and was never wired).
- **`Ctrl/Cmd+K`** → toggle the command palette.

Both call `preventDefault` so they don't fall through to the browser. Chosen to
avoid collision with the existing chat shortcuts (`Shift+Tab` = Plan/Act,
`Cmd/Ctrl+F` = search).

### Behavior / interface
- **Same dispatch, new triggers:** chips call the identical `onSlashCommand`
  path the typed `/command` already invokes; the palette reuses existing actions.
  One source of truth per action; we only add callers.
- Mobile-first: all new tap targets ≥ 44px, no hover dependency.

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
3. **Mobile reachability:** already handled — `MessageBubble` renders the action
   row `opacity-100` by default and only hides it behind hover at the `md:`
   breakpoint (`md:opacity-0 md:group-hover:opacity-100`). So on touch the actions
   are always visible; no change needed here. (This is why removing the unused
   Branch/Fork buttons is the actual win — they're always-on clutter on mobile.)

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
- **Build a new tap-to-open SlashCommandMenu** vs **wire up the existing
  CommandPalette** (chose the palette): planning found a complete, unwired
  `CommandPalette` already in the tree. Reusing it gives fuzzy search over
  commands *and* threads *and* actions for free — strictly more than a new menu —
  with less new code. We skip the equally-orphaned `useKeyboardShortcuts` hook
  (depends on a help modal we don't have) and bind only the two hotkeys we need.

## Testing

- **Frontend (Vitest):**
  - Chips dispatch the correct `onSlashCommand` value on click/tap (mock the
    prop, assert it's called with `/new` / `/find` / `/clear`) — one per chip.
  - `usePaletteHotkeys`: `Ctrl/Cmd+Shift+O` fires the new-chat callback;
    `Ctrl/Cmd+K` fires the palette-toggle callback (hook tested in isolation).
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
