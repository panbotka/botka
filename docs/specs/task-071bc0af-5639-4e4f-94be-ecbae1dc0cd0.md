Set up frontend testing infrastructure and write unit tests for key React components and hooks.

## Current state
- Backend has ~312 Go tests across 29 files
- Frontend has ZERO tests — no test runner, no test files
- This is a gap for future development — frontend changes are untested

## Requirements

### Set up test infrastructure
- Install Vitest (works natively with Vite, fast, Jest-compatible API)
- Install React Testing Library (`@testing-library/react`, `@testing-library/jest-dom`, `@testing-library/user-event`)
- Install `jsdom` for DOM environment
- Configure Vitest in `vite.config.ts` or `vitest.config.ts`
- Add `test` and `test:watch` scripts to `package.json`
- Add frontend test step to `make check` gate (so CI catches frontend test failures)

### Write tests for hooks (highest value, easiest to test)
- `useThreads` — test fetching, creating, deleting threads
- `useMessages` — test fetching messages, sending messages
- `useTasks` — test fetching, filtering tasks
- `useSettings` — test loading/saving settings
- Mock API calls with `vi.mock` or `msw` (Mock Service Worker)
- Test loading states, error states, success states

### Write tests for utility functions
- Any pure functions in `frontend/src/utils/` — format helpers, date utils, parsers
- These are the easiest to test — no mocking needed

### Write tests for key components
- `MessageBubble` — renders user vs assistant messages correctly, shows attachments
- `ToolCallPanel` — renders tool name, expands/collapses
- `BottomNav` — renders all tabs, highlights active tab
- `ThreadList` — renders threads, handles click
- Use snapshot tests sparingly — prefer behavioral assertions

### Test patterns
- Keep tests co-located: `Component.test.tsx` next to `Component.tsx`
- Use `describe/it` blocks
- Follow Arrange-Act-Assert pattern
- Mock API layer, not individual fetch calls
- Don't test implementation details — test behavior

## Implementation Notes
- Vitest is preferred over Jest for Vite projects — zero config, same API
- React Testing Library encourages testing from the user's perspective
- Start with hooks and utils (most testable), then components
- Don't aim for 100% coverage — focus on business logic and user interactions
- Existing `make check` must still pass — add the new test command to it

## Safety
**NEVER run `make deploy`, `make install-service`, `systemctl restart botka`, or any command that would restart the Botka service.**