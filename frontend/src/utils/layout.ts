// BOTTOM_NAV_HEIGHT is the full rendered height of the mobile BottomNav: its
// h-14 tab row, the 1px top border, and the home-indicator safe-area padding.
// Bottom-docked overlays (e.g. the task bulk-actions bar) offset by this so
// they sit directly above the nav instead of covering it. Keep in sync with
// components/BottomNav.tsx.
export const BOTTOM_NAV_HEIGHT = 'calc(3.5rem + 1px + env(safe-area-inset-bottom, 0px))'
