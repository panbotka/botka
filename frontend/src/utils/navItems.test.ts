import { describe, it, expect } from 'vitest'
import { NAV_ITEMS, isNavPathActive, navItemsFor } from './navItems'

describe('navItems', () => {
  it('orders the primary group Chat, Tasks, Box, Dashboard', () => {
    expect(navItemsFor('primary', true).map((i) => i.path)).toEqual(['/chat', '/tasks', '/box', '/'])
  })

  it('orders the secondary group Stats, Schedules, Cron, Settings, Help', () => {
    expect(navItemsFor('secondary', true).map((i) => i.path)).toEqual([
      '/stats',
      '/schedules',
      '/cron-jobs',
      '/settings',
      '/help',
    ])
  })

  it('hides admin-only items from non-admins', () => {
    expect(navItemsFor('primary', false).map((i) => i.path)).toEqual(['/chat'])
    expect(navItemsFor('secondary', false).map((i) => i.path)).toEqual(['/help'])
  })

  it('has unique paths', () => {
    expect(new Set(NAV_ITEMS.map((i) => i.path)).size).toBe(NAV_ITEMS.length)
  })

  it('matches the dashboard only exactly and others by subtree', () => {
    expect(isNavPathActive('/', '/')).toBe(true)
    expect(isNavPathActive('/', '/chat')).toBe(false)
    expect(isNavPathActive('/chat', '/chat/12')).toBe(true)
    expect(isNavPathActive('/tasks', '/tasks')).toBe(true)
    expect(isNavPathActive('/stats', '/statsx')).toBe(false)
  })
})
