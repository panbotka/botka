# Chat In-Flow Streamlining Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the chat faster to use — especially on mobile — by surfacing common commands without typing, removing unused branch/fork clutter, and making in-response choices keyboard-operable.

**Architecture:** Three independent slices. (A) Command access: reuse the already-built-but-unwired `CommandPalette` (wire it into `ChatPage`, open it via a visible button + `Cmd/Ctrl+K`), add a small `usePaletteHotkeys` hook for the new-chat shortcut, and add quick-action chips to the composer that reuse the existing `onSlashCommand` dispatch. (B) Declutter: remove the Branch and Fork buttons (and now-dead handlers/modal) from the message UI; backend endpoints stay. (C) Keyboard choices: add number-key selection to `SelectableOptions` and `AskUserPanel`.

**Tech Stack:** React 19 + TypeScript, Vite, Tailwind CSS 4, lucide-react icons, Vitest + @testing-library/react (jsdom), `vi.mock` for module mocks.

## Global Constraints

- **One source of truth per action:** chips and the palette must reuse existing
  dispatch (`ChatView.handleSlashCommand` via the `onSlashCommand` prop; the
  palette's existing action handlers). Do **not** re-implement what `/clear`,
  `/new`, etc. do.
- **No backend changes.** The branch/fork/active-branch HTTP endpoints and Go
  handlers stay untouched. Only frontend triggers are removed.
- **Mobile-first:** every new tap target ≥ 44px (or ≥ 36px for inline chips),
  no hover dependency for anything a mobile user needs.
- **`make check` must pass** after every task (Go fmt/vet/lint/test + frontend
  `tsc` type-check + Vitest). Frontend type errors from dead imports are failures.
- **Test runner:** `cd frontend && npx vitest run <path>` for a single file;
  `make check` for the full gate.
- Czech UI copy where the spec specifies it (`Nový chat`, `Hledat`,
  `Smazat historii`).

---

## File Structure

**Create:**
- `frontend/src/hooks/usePaletteHotkeys.ts` — global hotkeys: new chat + palette toggle.
- `frontend/src/hooks/usePaletteHotkeys.test.ts` — hook unit tests.
- `frontend/src/components/CommandPalette.test.tsx` — locks the palette contract.
- `frontend/src/components/ChatInput.chips.test.tsx` — chip dispatch tests.

**Modify:**
- `frontend/src/pages/ChatPage.tsx` — render `CommandPalette`, add palette state,
  call `usePaletteHotkeys`, add a visible palette-open button in both headers.
- `frontend/src/components/ChatInput.tsx` — add three quick-action chips.
- `frontend/src/components/MessageActions.tsx` — remove Branch + Fork buttons/props.
- `frontend/src/components/MessageBubble.tsx` — remove `onBranch`/`onFork`/`forkPoint`/`onSwitchBranch` props + `BranchIndicator` usage.
- `frontend/src/components/ChatView.tsx` — remove branch/fork handlers, state, banner, `ForkThreadModal` usage + import.
- `frontend/src/components/SelectableOptions.tsx` — number-key selection.
- `frontend/src/components/AskUserPanel.tsx` — number-key selection + Enter submit.

**Delete (after confirming dead):**
- `frontend/src/components/ForkThreadModal.tsx`

**Leave intact:** `ThreadForkBadges.tsx` (header navigation for already-existing
forked threads — not part of the per-message clutter), `api.forkThread` /
`api.switchBranch` / `api.branch` client methods (harmless; backend stays).

---

## Task 1: `usePaletteHotkeys` hook (new-chat shortcut + palette toggle)

**Files:**
- Create: `frontend/src/hooks/usePaletteHotkeys.ts`
- Test: `frontend/src/hooks/usePaletteHotkeys.test.ts`

**Interfaces:**
- Produces: `usePaletteHotkeys(opts: { onNewChat: () => void; onTogglePalette: () => void }): void`
  - Binds a global `keydown` listener while mounted.
  - `Ctrl/Cmd + Shift + O` → `onNewChat()` (with `preventDefault`).
  - `Ctrl/Cmd + K` (no shift/alt) → `onTogglePalette()` (with `preventDefault`).

- [ ] **Step 1: Write the failing test**

Create `frontend/src/hooks/usePaletteHotkeys.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { usePaletteHotkeys } from './usePaletteHotkeys'

function press(init: KeyboardEventInit) {
  window.dispatchEvent(new KeyboardEvent('keydown', { ...init, bubbles: true, cancelable: true }))
}

describe('usePaletteHotkeys', () => {
  it('fires onNewChat for Ctrl+Shift+O', () => {
    const onNewChat = vi.fn()
    const onTogglePalette = vi.fn()
    renderHook(() => usePaletteHotkeys({ onNewChat, onTogglePalette }))
    press({ key: 'O', ctrlKey: true, shiftKey: true })
    expect(onNewChat).toHaveBeenCalledTimes(1)
    expect(onTogglePalette).not.toHaveBeenCalled()
  })

  it('fires onNewChat for Cmd+Shift+O (meta)', () => {
    const onNewChat = vi.fn()
    renderHook(() => usePaletteHotkeys({ onNewChat, onTogglePalette: vi.fn() }))
    press({ key: 'o', metaKey: true, shiftKey: true })
    expect(onNewChat).toHaveBeenCalledTimes(1)
  })

  it('fires onTogglePalette for Cmd+K', () => {
    const onTogglePalette = vi.fn()
    renderHook(() => usePaletteHotkeys({ onNewChat: vi.fn(), onTogglePalette }))
    press({ key: 'k', metaKey: true })
    expect(onTogglePalette).toHaveBeenCalledTimes(1)
  })

  it('ignores plain O / plain K', () => {
    const onNewChat = vi.fn()
    const onTogglePalette = vi.fn()
    renderHook(() => usePaletteHotkeys({ onNewChat, onTogglePalette }))
    press({ key: 'o' })
    press({ key: 'k' })
    expect(onNewChat).not.toHaveBeenCalled()
    expect(onTogglePalette).not.toHaveBeenCalled()
  })

  it('unbinds on unmount', () => {
    const onTogglePalette = vi.fn()
    const { unmount } = renderHook(() => usePaletteHotkeys({ onNewChat: vi.fn(), onTogglePalette }))
    unmount()
    press({ key: 'k', metaKey: true })
    expect(onTogglePalette).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend && npx vitest run src/hooks/usePaletteHotkeys.test.ts`
Expected: FAIL — `Failed to resolve import './usePaletteHotkeys'`.

- [ ] **Step 3: Write the minimal implementation**

Create `frontend/src/hooks/usePaletteHotkeys.ts`:

```ts
import { useEffect } from 'react'

interface Options {
  onNewChat: () => void
  onTogglePalette: () => void
}

/**
 * Global keyboard shortcuts for the chat surface:
 * - Ctrl/Cmd + Shift + O → new chat
 * - Ctrl/Cmd + K        → toggle the command palette
 *
 * Deliberately small and self-contained: the repo also has an unwired
 * `useKeyboardShortcuts` hook, but it depends on a shortcuts-help modal we
 * do not render, so we bind only the two hotkeys we actually need here.
 */
export function usePaletteHotkeys({ onNewChat, onTogglePalette }: Options): void {
  useEffect(() => {
    function handler(e: KeyboardEvent) {
      const mod = e.ctrlKey || e.metaKey
      if (!mod || e.altKey) return

      if (e.shiftKey && (e.key === 'O' || e.key === 'o')) {
        e.preventDefault()
        onNewChat()
        return
      }
      if (!e.shiftKey && (e.key === 'K' || e.key === 'k')) {
        e.preventDefault()
        onTogglePalette()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onNewChat, onTogglePalette])
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd frontend && npx vitest run src/hooks/usePaletteHotkeys.test.ts`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/hooks/usePaletteHotkeys.ts frontend/src/hooks/usePaletteHotkeys.test.ts
git commit -m "Add usePaletteHotkeys hook (new chat + palette toggle)"
```

---

## Task 2: Wire CommandPalette into ChatPage + visible open button

The palette component already exists and is complete (`CommandPalette.tsx`, a
fixed overlay with internal Arrow/Enter/Escape handling). This task renders it,
feeds it props from existing `ChatPage` state, opens it via `usePaletteHotkeys`
and a visible button, and locks its contract with a test.

**Files:**
- Modify: `frontend/src/pages/ChatPage.tsx`
- Test: `frontend/src/components/CommandPalette.test.tsx`

**Interfaces:**
- Consumes: `usePaletteHotkeys` (Task 1); existing `CommandPalette` props
  (`open`, `onClose`, `threads`, `onSelectThread`, `onNewThread`,
  `onOpenSettings`, `onToggleTheme`, `onOpenSearch`).
- Consumes from `ChatPage`: `threads`, `selectThread`, `handleNewThread`,
  `setSettingsPanelOpen`, and `useSettings()` (`settings`, `updateSettings`).

- [ ] **Step 1: Write the failing test (palette contract)**

Create `frontend/src/components/CommandPalette.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import CommandPalette from './CommandPalette'

function renderPalette(overrides = {}) {
  const props = {
    open: true,
    onClose: vi.fn(),
    threads: [],
    onSelectThread: vi.fn(),
    onNewThread: vi.fn(),
    onOpenSettings: vi.fn(),
    onToggleTheme: vi.fn(),
    onOpenSearch: vi.fn(),
    ...overrides,
  }
  render(<CommandPalette {...props} />)
  return props
}

describe('CommandPalette', () => {
  it('renders nothing when closed', () => {
    const { container } = render(
      <CommandPalette
        open={false}
        onClose={vi.fn()}
        threads={[]}
        onSelectThread={vi.fn()}
        onNewThread={vi.fn()}
        onOpenSettings={vi.fn()}
        onToggleTheme={vi.fn()}
        onOpenSearch={vi.fn()}
      />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('shows the New chat action when open', () => {
    renderPalette()
    expect(screen.getByText('New chat')).toBeTruthy()
  })

  it('invokes onNewThread when New chat is activated', () => {
    const props = renderPalette()
    fireEvent.click(screen.getByText('New chat'))
    expect(props.onNewThread).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 2: Run the test to verify it passes (palette already works)**

Run: `cd frontend && npx vitest run src/components/CommandPalette.test.tsx`
Expected: PASS. (If "New chat" is not directly clickable because the row wraps
the label, target the row: `screen.getByText('New chat').closest('button, [role="option"]')` and click that. Adjust the selector to the palette's actual row element — inspect `CommandPalette.tsx` lines ~300-387 for the row wrapper — and keep the assertion that `onNewThread` fired.)

This step has no production code change; it documents and protects the contract
we are about to depend on. Commit it with Step 7.

- [ ] **Step 3: Add palette state + props wiring in ChatPage**

In `frontend/src/pages/ChatPage.tsx`:

Add the import near the other component imports (around line 15):

```tsx
import CommandPalette from '../components/CommandPalette'
import { usePaletteHotkeys } from '../hooks/usePaletteHotkeys'
import { useSettings } from '../context/SettingsContext'
```

Add state next to `settingsPanelOpen` (around line 42):

```tsx
const [paletteOpen, setPaletteOpen] = useState(false)
```

Add the settings hook near the top of the component body (with the other hooks):

```tsx
const { settings, updateSettings } = useSettings()
```

Wire the hotkeys (place after `handleNewThread` is defined, ~line 155):

```tsx
usePaletteHotkeys({
  onNewChat: () => handleNewThread(),
  onTogglePalette: () => setPaletteOpen((o) => !o),
})
```

- [ ] **Step 4: Render the palette once for both layouts**

Still in `ChatPage.tsx`, render `CommandPalette` so it is present in both the
mobile and desktop returns. The cleanest spot is just before the outermost
closing fragment/element of each returned tree. Add this block inside both the
mobile `return (...)` and the desktop `return (...)` (or factor a
`const palette = (...)` const above the returns and drop `{palette}` into each):

```tsx
<CommandPalette
  open={paletteOpen}
  onClose={() => setPaletteOpen(false)}
  threads={threads}
  onSelectThread={(id) => selectThread(id)}
  onNewThread={() => handleNewThread()}
  onOpenSettings={() => setSettingsPanelOpen(true)}
  onToggleTheme={() =>
    updateSettings({ theme: settings.theme === 'light' ? 'dark' : 'light' })
  }
  onOpenSearch={() => setPaletteOpen(false)}
/>
```

Note: `onOpenSearch` is intentionally a safe no-op-ish close for now (global
message search is not wired in chat today; the `/find` chip in Task 3 covers
in-thread search). Do not invent new search plumbing here.

- [ ] **Step 5: Add a visible palette-open button to both headers**

Mobile header (`ChatPage.tsx` ~line 220, the `<header>` with the back button and
title): add a button on the right side of that header:

```tsx
<button
  onClick={() => setPaletteOpen(true)}
  className="text-zinc-500 hover:text-zinc-800 transition-colors min-w-[44px] min-h-[44px] flex items-center justify-center"
  title="Příkazy"
  aria-label="Otevřít příkazy"
>
  <Command className="w-5 h-5" />
</button>
```

Desktop header (the header region around lines 360-372 where the thread
settings button lives): add the same button next to the settings button.

Add `Command` to the lucide import at the top of `ChatPage.tsx`:

```tsx
import { MessageSquare, ArrowLeft, Settings, Command } from 'lucide-react'
```

- [ ] **Step 6: Type-check and verify build**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS (no type errors). If `useSettings` is already imported elsewhere
in `ChatPage`, remove the duplicate import.

- [ ] **Step 7: Manual verification (mobile + desktop)**

Build is verified via tests/tsc; behavior is verified manually since `ChatPage`
is an integration surface:
- `cd frontend && npm run dev`, open the app.
- Desktop: press `Cmd/Ctrl+K` → palette opens; type to fuzzy-search; `Esc`
  closes. Press `Ctrl/Cmd+Shift+O` → a new thread is created.
- Mobile (DevTools device mode): tap the new command button in the header →
  palette opens; tap **New chat** → new thread.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/pages/ChatPage.tsx frontend/src/components/CommandPalette.test.tsx
git commit -m "Wire CommandPalette into ChatPage with hotkeys and a visible open button"
```

---

## Task 3: Quick-action chips in the composer

Three always-visible chips in the `ChatInput` toolbar that reuse the existing
`onSlashCommand` dispatch.

**Files:**
- Modify: `frontend/src/components/ChatInput.tsx`
- Test: `frontend/src/components/ChatInput.chips.test.tsx`

**Interfaces:**
- Consumes: existing `ChatInput` prop `onSlashCommand: (command: string, args: string) => void`.
- Behavior: chips call `onSlashCommand('/new', '')`, `onSlashCommand('/find', '')`,
  `onSlashCommand('/clear', '')`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/ChatInput.chips.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { SettingsProvider } from '../context/SettingsContext'
import ChatInput from './ChatInput'

// Avoid mic/MediaRecorder in jsdom.
vi.mock('../hooks/useVoiceInput', () => ({
  useVoiceInput: () => ({
    state: 'idle',
    toggle: vi.fn(),
    cancel: vi.fn(),
    isSupported: false,
    mode: 'whisper',
  }),
}))

function renderInput() {
  const onSlashCommand = vi.fn()
  render(
    <SettingsProvider>
      <ChatInput threadId={1} onSend={vi.fn()} onSlashCommand={onSlashCommand} />
    </SettingsProvider>,
  )
  return onSlashCommand
}

describe('ChatInput quick-action chips', () => {
  it('dispatches /new when Nový chat chip is clicked', () => {
    const onSlash = renderInput()
    fireEvent.click(screen.getByLabelText('Nový chat'))
    expect(onSlash).toHaveBeenCalledWith('/new', '')
  })

  it('dispatches /find when Hledat chip is clicked', () => {
    const onSlash = renderInput()
    fireEvent.click(screen.getByLabelText('Hledat'))
    expect(onSlash).toHaveBeenCalledWith('/find', '')
  })

  it('dispatches /clear when Smazat historii chip is clicked', () => {
    const onSlash = renderInput()
    fireEvent.click(screen.getByLabelText('Smazat historii'))
    expect(onSlash).toHaveBeenCalledWith('/clear', '')
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend && npx vitest run src/components/ChatInput.chips.test.tsx`
Expected: FAIL — `Unable to find a label with the text of: Nový chat`.
(If it instead errors on a missing browser API from a non-mocked dependency,
extend the mock minimally to satisfy that import — do not change production code.)

- [ ] **Step 3: Add the chips to the toolbar**

In `frontend/src/components/ChatInput.tsx`, add the lucide import to the top
import block:

```tsx
import { Plus, Search, Trash2 } from 'lucide-react'
```

In the composer toolbar region (the row holding the upload/voice buttons, around
lines 333-345 — next to the file-upload button), insert the three chips:

```tsx
{/* Quick-action chips — reuse the existing slash-command dispatch */}
<button
  type="button"
  onClick={() => onSlashCommand('/new', '')}
  className="flex items-center gap-1 px-2 min-h-[36px] rounded-md text-zinc-500 hover:text-zinc-800 hover:bg-zinc-100 text-xs transition-colors"
  title="Nový chat"
  aria-label="Nový chat"
>
  <Plus size={16} />
  <span className="hidden sm:inline">Nový</span>
</button>
<button
  type="button"
  onClick={() => onSlashCommand('/find', '')}
  className="flex items-center gap-1 px-2 min-h-[36px] rounded-md text-zinc-500 hover:text-zinc-800 hover:bg-zinc-100 text-xs transition-colors"
  title="Hledat"
  aria-label="Hledat"
>
  <Search size={16} />
  <span className="hidden sm:inline">Hledat</span>
</button>
<button
  type="button"
  onClick={() => onSlashCommand('/clear', '')}
  className="flex items-center gap-1 px-2 min-h-[36px] rounded-md text-zinc-500 hover:text-zinc-800 hover:bg-zinc-100 text-xs transition-colors"
  title="Smazat historii"
  aria-label="Smazat historii"
>
  <Trash2 size={16} />
  <span className="hidden sm:inline">Smazat</span>
</button>
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd frontend && npx vitest run src/components/ChatInput.chips.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 5: Manual sanity check**

`cd frontend && npm run dev`; in a thread, tap **Nový** (new thread), **Hledat**
(in-thread search opens), **Smazat** (clears with the existing confirmation).
On a narrow viewport the chips collapse to icon-only and stay tappable.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/ChatInput.tsx frontend/src/components/ChatInput.chips.test.tsx
git commit -m "Add quick-action chips (Nový chat / Hledat / Smazat historii) to composer"
```

---

## Task 4: Remove unused Branch + Fork from the message UI

Branch and Fork are never used and clutter every message (always visible on
mobile). Remove their entry points and the now-dead handlers/state/modal.
Backend endpoints stay.

**Files:**
- Modify: `frontend/src/components/MessageActions.tsx`
- Modify: `frontend/src/components/MessageBubble.tsx`
- Modify: `frontend/src/components/ChatView.tsx`
- Delete: `frontend/src/components/ForkThreadModal.tsx`
- Test: extend/replace `frontend/src/components/MessageBubble.test.tsx` or add
  `frontend/src/components/MessageActions.test.tsx` (new).

**Interfaces:**
- After this task, `MessageActions` `Props` no longer has `onBranch` or `onFork`.
- `MessageBubble` `Props` no longer has `onBranch`, `onFork`, `forkPoint`,
  `onSwitchBranch`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/MessageActions.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import MessageActions from './MessageActions'

describe('MessageActions', () => {
  it('does not render Branch or Fork actions for an assistant message', () => {
    render(
      <MessageActions
        role="assistant"
        content="hi"
        isLastAssistant={false}
        onRegenerate={vi.fn()}
        onHide={vi.fn()}
      />,
    )
    expect(screen.queryByLabelText('Branch from here')).toBeNull()
    expect(screen.queryByLabelText('Fork thread from here')).toBeNull()
  })

  it('still renders Copy', () => {
    render(
      <MessageActions role="assistant" content="hi" isLastAssistant={false} />,
    )
    // Copy button has aria-label "Copy" (confirm the exact label in the file).
    expect(screen.getByLabelText('Copy')).toBeTruthy()
  })
})
```

(Confirm the Copy button's exact `aria-label` in `MessageActions.tsx` lines
40-56 and match it. If it differs, update the assertion string.)

- [ ] **Step 2: Run the test to verify the current failure**

Run: `cd frontend && npx vitest run src/components/MessageActions.test.tsx`
Expected: FAIL on the first test — Branch/Fork are still rendered when their
callbacks are passed. (Note: this test omits `onBranch`/`onFork`, so it may
actually pass already for assistant-not-last; the real protection is the
type-level removal in Step 3 plus this regression guard. If it passes now,
that's fine — proceed to remove the buttons and keep the test green.)

- [ ] **Step 3: Remove Branch + Fork from `MessageActions.tsx`**

Delete the Branch button block (lines ~71-82) and the Fork button block
(lines ~97-106). Remove `onBranch?` and `onFork?` from the `Props` interface
(lines 4-14). The remaining buttons (Copy, Edit, Regenerate, Hide) are unchanged.

Resulting `Props`:

```tsx
interface Props {
  role: 'user' | 'assistant' | 'system'
  content: string
  isLastAssistant: boolean
  isHidden?: boolean
  onEdit?: () => void
  onRegenerate?: () => void
  onHide?: () => void
}
```

- [ ] **Step 4: Remove the props from `MessageBubble.tsx`**

In `frontend/src/components/MessageBubble.tsx`:
- Remove `onBranch?`, `onFork?`, `forkPoint?`, and `onSwitchBranch?` from the
  `Props` interface (lines ~20-23).
- Remove `onBranch={onBranch}` and `onFork={onFork}` from the `<MessageActions>`
  usage (lines ~438-439).
- Remove the `BranchIndicator` block (lines ~443-445):
  `{forkPoint && onSwitchBranch && (<BranchIndicator ... />)}`.
- Remove the now-unused `BranchIndicator` import.
- Remove these names from the destructured function params (line ~225).

- [ ] **Step 5: Remove dead branch/fork code from `ChatView.tsx`**

In `frontend/src/components/ChatView.tsx`:
- Remove state: `forkPoints` (line 50), `branchFromId` (line 53),
  `forkFromMessage` (line 56).
- Remove handlers: `handleBranch` (679-683), `handleSwitchBranch` (685-693),
  `handleForkRequest` (697-701), `handleForkConfirm` (703-708).
- Remove the `onBranch=`, `onFork=`, `onSwitchBranch=` props passed to
  `<MessageBubble>` (lines ~888-908) and any `fp`/`forkId`/`forkPoint` lookups
  that fed them.
- Remove the branching banner block (1088-1106, the `branchFromId != null` JSX).
- Remove the `ForkThreadModal` usage block (1175-1182) and its import (line 17).

- [ ] **Step 6: Delete `ForkThreadModal.tsx`**

```bash
git rm frontend/src/components/ForkThreadModal.tsx
```

- [ ] **Step 7: Type-check (catches every remaining dangling reference)**

Run: `cd frontend && npx tsc --noEmit`
Expected: PASS. Fix any remaining references the compiler flags (e.g. an
unused import, a leftover `forkPoints` read). Iterate until clean — `tsc` is
the safety net that proves the removal is complete.

- [ ] **Step 8: Run the message tests**

Run: `cd frontend && npx vitest run src/components/MessageActions.test.tsx src/components/MessageBubble.test.tsx`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add -A frontend/src/components/MessageActions.tsx frontend/src/components/MessageBubble.tsx frontend/src/components/ChatView.tsx frontend/src/components/MessageActions.test.tsx
git commit -m "Remove unused Branch/Fork actions from chat message UI"
```

---

## Task 5: Number-key selection in `SelectableOptions`

**Files:**
- Modify: `frontend/src/components/SelectableOptions.tsx`
- Test: `frontend/src/components/SelectableOptions.test.tsx` (new)

**Interfaces:**
- Unchanged `Props`: `{ children: ReactNode; onSelect: (text: string) => void }`.
- Adds: pressing `1`–`9` selects the option at that 1-based index immediately.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/SelectableOptions.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/react'
import SelectableOptions from './SelectableOptions'

describe('SelectableOptions', () => {
  it('selects the option matching a pressed number key', () => {
    const onSelect = vi.fn()
    const { container } = render(
      <SelectableOptions onSelect={onSelect}>
        <ol>
          <li>First option</li>
          <li>Second option</li>
        </ol>
      </SelectableOptions>,
    )
    // The component attaches its keydown handler to its focusable root.
    const root = container.firstChild as HTMLElement
    root.focus()
    fireEvent.keyDown(root, { key: '2' })
    expect(onSelect).toHaveBeenCalledWith('Second option')
  })
})
```

(Confirm how the root receives key events in `SelectableOptions.tsx` — it uses a
`handleKeyDown` bound to its container at lines 27-39. Target that container in
the test; adjust the selector if the handler is on a different element.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend && npx vitest run src/components/SelectableOptions.test.tsx`
Expected: FAIL — `onSelect` not called for key `2`.

- [ ] **Step 3: Add number-key handling**

In `frontend/src/components/SelectableOptions.tsx`, inside `handleKeyDown`
(lines 27-39), add a branch after the ArrowUp case:

```tsx
} else if (e.key >= '1' && e.key <= '9') {
  const idx = parseInt(e.key, 10) - 1
  if (idx < items.length) {
    e.preventDefault()
    const text = extractText(items[idx])
    if (text.trim()) onSelect(text.trim())
  }
}
```

(Insert it as an `else if` in the existing `if (e.key === 'ArrowDown') {...}
else if (e.key === 'ArrowUp') {...} else if (e.key === 'Enter' ...) {...}`
chain, before or after the Enter branch — order does not matter since the keys
are disjoint.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd frontend && npx vitest run src/components/SelectableOptions.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/SelectableOptions.tsx frontend/src/components/SelectableOptions.test.tsx
git commit -m "Add number-key selection to SelectableOptions"
```

---

## Task 6: Number-key selection + Enter submit in `AskUserPanel`

**Files:**
- Modify: `frontend/src/components/AskUserPanel.tsx`
- Test: `frontend/src/components/AskUserPanel.test.tsx` (new)

**Interfaces:**
- `Props` unchanged (`{ toolCall: ActiveToolCall; threadId: number }`).
- For a single-question, options-based prompt: pressing `1`–`9` selects the
  option at that index (same as clicking it). `Enter` (no Shift) still submits
  via the existing `handleSubmit`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/AskUserPanel.test.tsx`. Mock the network
submit so the test stays unit-level:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'

const submitToolResult = vi.fn().mockResolvedValue(undefined)
vi.mock('../api/client', () => ({
  api: {},
  submitToolResult: (...args: unknown[]) => submitToolResult(...args),
}))

import AskUserPanel from './AskUserPanel'

// Minimal tool call with one single-select question.
const toolCall = {
  id: 'tc1',
  name: 'AskUserQuestion',
  input: {
    questions: [
      { question: 'Pick one', header: 'X', multiSelect: false, options: [
        { label: 'Alpha', description: '' },
        { label: 'Beta', description: '' },
      ] },
    ],
  },
} as never

describe('AskUserPanel', () => {
  it('selects an option when its number key is pressed', () => {
    render(<AskUserPanel toolCall={toolCall} threadId={1} />)
    fireEvent.keyDown(window, { key: '2' })
    // The Beta option button becomes visually selected — assert via aria-pressed
    // or the selected class the panel applies (confirm the exact attribute in
    // AskUserPanel.tsx lines 159-185 and match it here).
    const beta = screen.getByText('Beta').closest('button')!
    expect(beta.getAttribute('aria-pressed')).toBe('true')
  })
})
```

(Inspect `AskUserPanel.tsx` to confirm: the exact import name for the submit
function — adjust the `vi.mock` to match; whether selected options expose
`aria-pressed` or a class — if there is no `aria-pressed`, add one in Step 3 for
testability, or assert on the applied selected class instead.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend && npx vitest run src/components/AskUserPanel.test.tsx`
Expected: FAIL — number key does nothing yet.

- [ ] **Step 3: Add number-key selection to the panel**

In `frontend/src/components/AskUserPanel.tsx`, extend the existing
`handleKeyDown` (lines 115-120, currently Enter→submit). The panel already binds
keyboard handling for Enter; reuse the same handler (ensure it is attached at a
scope that receives key events even when no textarea is focused — bind a
`window` keydown effect for the number keys, scoped to a single options-based
question to keep it unambiguous):

```tsx
useEffect(() => {
  function onKey(e: KeyboardEvent) {
    // Only handle bare number keys, and only for a single options question,
    // so multi-question prompts stay unambiguous.
    if (e.ctrlKey || e.metaKey || e.altKey) return
    if (questions.length !== 1) return
    const q = questions[0]!
    if (!q.options || q.options.length === 0) return
    if (e.key >= '1' && e.key <= '9') {
      const idx = parseInt(e.key, 10) - 1
      if (idx < q.options.length) {
        e.preventDefault()
        toggleOption(0, q.options[idx]!.label)
      }
    }
  }
  window.addEventListener('keydown', onKey)
  return () => window.removeEventListener('keydown', onKey)
}, [questions, toggleOption])
```

If the option buttons (lines 159-185) do not already expose selection state to
assistive tech, add `aria-pressed={isSelected}` to each option button so the
test (and screen readers) can observe selection. `Enter`-to-submit already exists
via `handleKeyDown` (115-120) and is unchanged.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd frontend && npx vitest run src/components/AskUserPanel.test.tsx`
Expected: PASS.

- [ ] **Step 5: Manual check**

In a chat where Claude calls `AskUserQuestion` with a single options question,
press `1`/`2`/… to select and `Enter` to submit — no mouse.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/AskUserPanel.tsx frontend/src/components/AskUserPanel.test.tsx
git commit -m "Add number-key selection to AskUserPanel"
```

---

## Final: Full gate

- [ ] **Run the full CI gate**

Run: `make check`
Expected: PASS — Go fmt/vet/lint/test + frontend `tsc` + Vitest all green.
**Do not run `make deploy` or restart the botka service** (per repo CLAUDE.md —
it would kill a running task agent). Build/test only; the user deploys.

---

## Self-Review (completed during planning)

**Spec coverage:**
- Spec A1 (chips) → Task 3. A2 (palette reuse + mobile open button) → Task 2.
  A3 (new-chat shortcut `Ctrl/Cmd+Shift+O`, plus `Cmd/Ctrl+K`) → Task 1 + Task 2.
- Spec B (remove Branch/Fork; mobile already shows actions) → Task 4. (Mobile
  reachability needed no code — `opacity-100` below `md`.)
- Spec C1 (SelectableOptions number keys) → Task 5. C2 (AskUserPanel number keys
  + Enter submit) → Task 6.
- Spec "no backend changes / endpoints remain" → enforced in Global Constraints
  and Task 4 (UI-only removal).

**Placeholders:** none — every code step shows real code; selector/aria-label
caveats point at exact line ranges to confirm against, not vague TODOs.

**Type consistency:** `usePaletteHotkeys({ onNewChat, onTogglePalette })` is
defined in Task 1 and consumed verbatim in Task 2. `CommandPalette` props match
the component's real interface (lines 5-14). `MessageActions`/`MessageBubble`
prop removals are mirrored across producer and consumer in Task 4.

**Scope:** one cohesive plan; A→B→C are independent and each leaves
`make check` green, so they can land (and be reviewed) in sequence.
