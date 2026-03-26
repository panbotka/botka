In dark mode, several UI elements on the Settings page have a white/light background instead of dark. This makes them look broken and unreadable.

## Screenshot analysis
The following elements on the Settings page have incorrect white backgrounds in dark mode:
- **Theme toggle buttons** (Light, [Dark], Dark Green, Dark Blue, System) — the button group has white background, text is barely visible on selected state
- **Font Size buttons** (Small, [Medium], Large) — same issue, white button group background
- The **Default AI Model** dropdown also appears to have a white background

The rest of the page (labels, toggles, bottom nav) renders correctly in dark mode.

## Expected behavior
- All interactive elements (buttons, selects, dropdowns) should use dark backgrounds in dark mode
- Button groups: dark background (e.g. `bg-zinc-800`), with selected button having a slightly lighter or accented background
- Dropdowns/selects: dark background with light text
- Follow the existing dark mode pattern used elsewhere in the app

## Requirements

### Fix button group backgrounds
- Find the button group components in the Settings page (SettingsPage.tsx or sub-components)
- Add dark mode variants: `bg-white dark:bg-zinc-800` or similar
- Selected state: ensure contrast — e.g. `bg-zinc-200 dark:bg-zinc-700` for selected, `bg-white dark:bg-zinc-800` for unselected
- Button text: `text-zinc-900 dark:text-zinc-100`
- Button borders: `border-zinc-200 dark:border-zinc-700`

### Fix select/dropdown backgrounds
- The model dropdown should have `bg-white dark:bg-zinc-800` with `text-zinc-900 dark:text-zinc-100`

### Audit the rest of Settings
- Check all four tabs (General, Task Runner, Personas, Tags) for similar issues
- Any form inputs, buttons, cards, or interactive elements should have proper dark mode styles
- Compare with other pages that work correctly in dark mode and use the same pattern

### General dark mode audit
- While fixing Settings, also quickly scan other pages for similar white-background-in-dark-mode issues
- Common culprits: `bg-white` without `dark:bg-zinc-800/900`, hardcoded colors without dark variants

## Implementation Notes
- Tailwind dark mode is likely class-based (`dark:` prefix)
- Search for `bg-white` in the Settings page and ensure each has a `dark:bg-zinc-*` counterpart
- Search for `text-black` or `text-zinc-900` and add `dark:text-zinc-100` etc.
- Existing tests must pass (`make check`)

## Safety
**NEVER run `make deploy`, `make install-service`, `systemctl restart botka`, or any command that would restart the Botka service.**