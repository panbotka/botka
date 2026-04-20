# Hide Left Sidebar on Desktop

Allow users to collapse/hide the left navigation sidebar on desktop for more screen space.

## Requirements

- On desktop viewport (same breakpoint that currently shows the left sidebar), render a small toggle control on the sidebar's right edge (arrow-style chevron).
- Clicking the toggle collapses the sidebar, leaving the main content area to fill the freed space.
- When collapsed, a corresponding control must remain visible so the user can reopen the sidebar.
- Collapsed/expanded state is in-memory only — it resets on page reload (no localStorage, no cookie, no DB).
- Arrow icon direction reflects current state (points left when expanded, right when collapsed, or equivalent).
- Mobile layout (bottom navigation bar) must not be affected in any way — no new controls, no layout shifts, no behavior changes.
- Transition between states should be smooth (CSS transition acceptable), without layout jank in the main content.

## Implementation Notes

- Sidebar and layout live in `frontend/src/components/` / layout components — reuse existing Tailwind patterns.
- Use the same breakpoint the app already uses to switch between desktop sidebar and mobile bottom nav; do not introduce a new one.
- Keep state local to the layout component (React `useState`) — no global store needed.