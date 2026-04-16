import { useLocation, useNavigate } from 'react-router-dom'
import { LayoutDashboard, MessageSquare, ListTodo, Timer, Server, Settings, HelpCircle } from 'lucide-react'
import { clsx } from 'clsx'
import { useAuth } from '../context/AuthContext'

const allTabs = [
  { path: '/', icon: LayoutDashboard, label: 'Dashboard', adminOnly: true },
  { path: '/chat', icon: MessageSquare, label: 'Chat', adminOnly: false },
  { path: '/tasks', icon: ListTodo, label: 'Tasks', adminOnly: true },
  { path: '/cron-jobs', icon: Timer, label: 'Cron', adminOnly: true },
  { path: '/box', icon: Server, label: 'Box', adminOnly: true },
  { path: '/settings', icon: Settings, label: 'Settings', adminOnly: true },
  { path: '/help', icon: HelpCircle, label: 'Help', adminOnly: false },
] as const

export default function BottomNav() {
  const location = useLocation()
  const navigate = useNavigate()
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const tabs = allTabs.filter((tab) => !tab.adminOnly || isAdmin)

  const isActive = (path: string) => {
    if (path === '/') return location.pathname === '/'
    return location.pathname.startsWith(path)
  }

  return (
    <nav
      className="fixed bottom-0 left-0 right-0 z-40 bg-zinc-50 border-t border-zinc-200 dark:bg-zinc-100 dark:border-zinc-300"
      style={{ paddingBottom: 'env(safe-area-inset-bottom, 0px)' }}
    >
      <div className="flex h-14">
        {tabs.map((tab) => (
          <button
            key={tab.path}
            onClick={() => navigate(tab.path)}
            className={clsx(
              'flex-1 flex flex-col items-center justify-center gap-0.5 transition-colors cursor-pointer min-h-[44px]',
              isActive(tab.path) ? 'text-amber-600' : 'text-zinc-400 active:text-zinc-600 dark:text-zinc-500',
            )}
          >
            <tab.icon className="w-6 h-6" />
            <span className="text-[10px] font-medium">{tab.label}</span>
          </button>
        ))}
      </div>
    </nav>
  )
}
