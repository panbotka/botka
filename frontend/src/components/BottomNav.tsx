import { useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { MoreHorizontal } from 'lucide-react'
import { clsx } from 'clsx'
import { useAuth } from '../context/AuthContext'
import { isNavPathActive, navItemsFor } from '../utils/navItems'
import BottomSheet from './BottomSheet'

export default function BottomNav() {
  const location = useLocation()
  const navigate = useNavigate()
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const primary = navItemsFor('primary', isAdmin)
  const secondary = navItemsFor('secondary', isAdmin)
  const [moreOpen, setMoreOpen] = useState(false)

  const isActive = (path: string) => isNavPathActive(path, location.pathname)
  const moreActive = moreOpen || secondary.some((item) => isActive(item.path))

  // Close the sheet whenever the route changes (item tap, back button, push
  // notification navigation…).
  useEffect(() => {
    setMoreOpen(false)
  }, [location.pathname])

  const go = (path: string) => {
    setMoreOpen(false)
    navigate(path)
  }

  const slotClass = (active: boolean) =>
    clsx(
      'flex-1 flex flex-col items-center justify-center gap-1 transition-colors cursor-pointer min-h-[44px]',
      active ? 'text-amber-600' : 'text-zinc-500 active:text-zinc-700 dark:text-zinc-500',
    )

  return (
    <>
      <BottomSheet
        open={moreOpen}
        onClose={() => setMoreOpen(false)}
        ariaLabel="Více"
        backdropTestId="more-backdrop"
        zIndexClass="z-40"
      >
        <ul className="p-2">
          {secondary.map((item) => (
            <li key={item.path}>
              <button
                type="button"
                onClick={() => go(item.path)}
                aria-current={isActive(item.path) ? 'page' : undefined}
                className={clsx(
                  'flex w-full min-h-[48px] items-center gap-3 rounded-lg px-4 text-left text-base font-medium transition-colors cursor-pointer',
                  isActive(item.path)
                    ? 'bg-amber-50 text-amber-700 dark:bg-amber-100/20'
                    : 'text-zinc-700 active:bg-zinc-100',
                )}
              >
                <item.icon className="h-5 w-5" />
                {item.label}
              </button>
            </li>
          ))}
        </ul>
      </BottomSheet>
      <nav
        className="flex-shrink-0 bg-zinc-50 border-t border-zinc-200 dark:bg-zinc-100 dark:border-zinc-300"
        style={{ paddingBottom: 'env(safe-area-inset-bottom, 0px)' }}
      >
        {/* Height mirrored in utils/layout.ts (BOTTOM_NAV_HEIGHT) — keep in sync. */}
        <div className="flex h-14">
          {primary.map((item) => (
            <button
              key={item.path}
              type="button"
              onClick={() => go(item.path)}
              aria-current={isActive(item.path) ? 'page' : undefined}
              className={slotClass(isActive(item.path))}
            >
              <item.icon className="w-6 h-6" />
              <span className="text-xs font-medium leading-none">{item.shortLabel ?? item.label}</span>
            </button>
          ))}
          {secondary.length > 0 && (
            <button
              type="button"
              onClick={() => setMoreOpen((prev) => !prev)}
              aria-haspopup="dialog"
              aria-expanded={moreOpen}
              className={slotClass(moreActive)}
            >
              <MoreHorizontal className="w-6 h-6" />
              <span className="text-xs font-medium leading-none">Více</span>
            </button>
          )}
        </div>
      </nav>
    </>
  )
}
