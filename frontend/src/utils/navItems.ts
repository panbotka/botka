import {
  LayoutDashboard,
  MessageSquare,
  ListTodo,
  Timer,
  CalendarClock,
  Server,
  Settings,
  HelpCircle,
  BarChart3,
  type LucideIcon,
} from 'lucide-react'

// NavGroup splits navigation into the everyday pages (primary — bottom-nav
// slots on mobile, top of the sidebar on desktop) and the rarely used ones
// (secondary — the "Více" sheet on mobile, the "Ostatní" group on desktop).
export type NavGroup = 'primary' | 'secondary'

export interface NavItem {
  path: string
  label: string
  // shortLabel replaces label in the narrow mobile bottom-nav slots.
  shortLabel?: string
  icon: LucideIcon
  adminOnly: boolean
  group: NavGroup
}

// NAV_ITEMS is the single source of truth for both BottomNav and the desktop
// sidebar, so the two can't drift apart. Order within a group is display order.
export const NAV_ITEMS: readonly NavItem[] = [
  { path: '/chat', label: 'Chat', icon: MessageSquare, adminOnly: false, group: 'primary' },
  { path: '/tasks', label: 'Tasks', shortLabel: 'Tasky', icon: ListTodo, adminOnly: true, group: 'primary' },
  { path: '/box', label: 'Box', icon: Server, adminOnly: true, group: 'primary' },
  { path: '/', label: 'Dashboard', icon: LayoutDashboard, adminOnly: true, group: 'primary' },
  { path: '/stats', label: 'Stats', icon: BarChart3, adminOnly: true, group: 'secondary' },
  { path: '/schedules', label: 'Schedules', icon: CalendarClock, adminOnly: true, group: 'secondary' },
  { path: '/cron-jobs', label: 'Cron Jobs', shortLabel: 'Cron', icon: Timer, adminOnly: true, group: 'secondary' },
  { path: '/settings', label: 'Settings', icon: Settings, adminOnly: true, group: 'secondary' },
  { path: '/help', label: 'Help', icon: HelpCircle, adminOnly: false, group: 'secondary' },
]

// navItemsFor returns the items of one group the user is allowed to see.
export function navItemsFor(group: NavGroup, isAdmin: boolean): NavItem[] {
  return NAV_ITEMS.filter((item) => item.group === group && (!item.adminOnly || isAdmin))
}

// isNavPathActive reports whether pathname belongs to the nav item at path.
// The dashboard ("/") matches only exactly; everything else matches its
// subtree (e.g. /chat/123 activates Chat).
export function isNavPathActive(path: string, pathname: string): boolean {
  if (path === '/') return pathname === '/'
  return pathname === path || pathname.startsWith(path + '/')
}
