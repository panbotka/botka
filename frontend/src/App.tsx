import { lazy, Suspense, useState, useEffect, useCallback } from 'react'
import { Routes, Route, NavLink, Navigate, useLocation, useNavigate } from 'react-router-dom'
import {
  LayoutDashboard,
  MessageSquare,
  ListTodo,
  Timer,
  CalendarClock,
  Settings,
  HelpCircle,
  Loader2,
  Server,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react'
import { clsx } from 'clsx'
import { useIsMobile } from './hooks/useIsMobile'
import { SSEProvider } from './context/SSEContext'
import { AuthProvider, useAuth } from './context/AuthContext'
import ErrorBoundary from './components/ErrorBoundary'
import BottomNav from './components/BottomNav'
import OfflineIndicator from './components/OfflineIndicator'
import UpdateBanner from './components/UpdateBanner'
import SearchOverlay from './components/SearchOverlay'

const DashboardPage = lazy(() => import('./pages/DashboardPage'))
const ChatPage = lazy(() => import('./pages/ChatPage'))
const TasksPage = lazy(() => import('./pages/TasksPage'))
const TaskDetailPage = lazy(() => import('./pages/TaskDetailPage'))
const CronJobsPage = lazy(() => import('./pages/CronJobsPage'))
const SchedulesPage = lazy(() => import('./pages/SchedulesPage'))
const ProjectDetailPage = lazy(() => import('./pages/ProjectDetailPage'))
const SettingsPage = lazy(() => import('./pages/SettingsPage'))
const HelpPage = lazy(() => import('./pages/HelpPage'))
const BoxPage = lazy(() => import('./pages/BoxPage'))
const LoginPage = lazy(() => import('./pages/LoginPage'))

const allNavItems = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard', adminOnly: true },
  { to: '/chat', icon: MessageSquare, label: 'Chat', adminOnly: false },
  { to: '/tasks', icon: ListTodo, label: 'Tasks', adminOnly: true },
  { to: '/schedules', icon: CalendarClock, label: 'Schedules', adminOnly: true },
  { to: '/cron-jobs', icon: Timer, label: 'Cron Jobs', adminOnly: true },
  { to: '/box', icon: Server, label: 'Box', adminOnly: true },
  { to: '/settings', icon: Settings, label: 'Settings', adminOnly: true },
  { to: '/help', icon: HelpCircle, label: 'Help', adminOnly: false },
] as const

function PageLoader() {
  return (
    <div className="flex h-64 items-center justify-center">
      <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
    </div>
  )
}

function FullPageLoader() {
  return (
    <div className="flex h-dvh items-center justify-center bg-zinc-50">
      <Loader2 className="h-8 w-8 animate-spin text-zinc-400" />
    </div>
  )
}

function AppSidebar() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const navItems = allNavItems.filter((item) => !item.adminOnly || isAdmin)

  return (
    <aside className="flex h-screen w-56 flex-col border-r border-zinc-200 bg-zinc-50 flex-shrink-0">
      <div className="flex h-14 items-center gap-2 border-b border-zinc-200 px-4">
        <span className="text-xl">🤖</span>
        <span className="text-lg font-semibold text-zinc-900">Botka</span>
      </div>
      <nav className="flex flex-1 flex-col gap-1 p-2">
        {navItems.map(({ to, icon: Icon, label }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            className={({ isActive }) =>
              clsx(
                'flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                isActive
                  ? 'bg-zinc-200 text-zinc-900 dark:bg-zinc-300'
                  : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 dark:hover:bg-zinc-200',
              )
            }
          >
            <Icon className="h-4 w-4" />
            {label}
          </NavLink>
        ))}
      </nav>
    </aside>
  )
}

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading, user } = useAuth()
  const location = useLocation()

  if (isLoading) return <FullPageLoader />
  if (!isAuthenticated) return <Navigate to="/login" state={{ from: location.pathname }} replace />

  // Redirect external users from admin-only pages to /chat.
  if (user?.role === 'external') {
    const path = location.pathname
    const allowedPaths = ['/chat', '/help']
    const isAllowed = allowedPaths.some((p) => path === p || path.startsWith(p + '/'))
    if (!isAllowed) {
      return <Navigate to="/chat" replace />
    }
  }

  return <>{children}</>
}

function AuthenticatedApp() {
  const location = useLocation()
  const navigate = useNavigate()
  const isMobile = useIsMobile()
  const isChat = location.pathname.startsWith('/chat')
  // Hide bottom nav when viewing an active chat thread on mobile
  const isActiveChatThread = /^\/chat\/\d+$/.test(location.pathname)
  const hideBottomNav = isMobile && isActiveChatThread

  const [searchOpen, setSearchOpen] = useState(false)
  const [searchInitialQuery, setSearchInitialQuery] = useState<string | undefined>(undefined)
  const closeSearch = useCallback(() => {
    setSearchOpen(false)
    setSearchInitialQuery(undefined)
  }, [])
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)

  // Global Cmd+K / Ctrl+K shortcut
  useEffect(() => {
    function handler(e: KeyboardEvent) {
      if (e.key === 'k' && (e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey) {
        e.preventDefault()
        setSearchInitialQuery(undefined)
        setSearchOpen((prev) => !prev)
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  // Listen for "open global search prefilled" events dispatched from sidebar etc.
  useEffect(() => {
    function handler(e: Event) {
      const ce = e as CustomEvent<{ query?: string }>
      setSearchInitialQuery(ce.detail?.query ?? '')
      setSearchOpen(true)
    }
    window.addEventListener('botka:open-search', handler as EventListener)
    return () => window.removeEventListener('botka:open-search', handler as EventListener)
  }, [])

  // Service-worker driven navigation: when the user clicks a Web Push
  // notification while a tab is open, the SW focuses it and posts a
  // {type:'navigate', url} message we honor here via react-router.
  useEffect(() => {
    if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return
    function handler(event: MessageEvent) {
      const data = event.data as { type?: string; url?: string } | undefined
      if (data?.type === 'navigate' && typeof data.url === 'string') {
        try {
          const target = new URL(data.url, window.location.origin)
          if (target.origin === window.location.origin) {
            navigate(target.pathname + target.search + target.hash)
          }
        } catch {
          // ignore malformed URLs
        }
      }
    }
    navigator.serviceWorker.addEventListener('message', handler)
    return () => navigator.serviceWorker.removeEventListener('message', handler)
  }, [navigate])

  return (
    <SSEProvider>
      <div className="relative flex h-dvh bg-zinc-50" style={{ paddingTop: 'env(safe-area-inset-top, 0px)' }}>
        {!isMobile && (
          <>
            <div
              className={clsx(
                'h-full flex-shrink-0 overflow-hidden transition-[width] duration-200 ease-in-out',
                sidebarCollapsed ? 'w-0' : 'w-56',
              )}
            >
              <div className="h-full w-56">
                <AppSidebar />
              </div>
            </div>
            <button
              type="button"
              onClick={() => setSidebarCollapsed((prev) => !prev)}
              aria-label={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
              className={clsx(
                'absolute top-4 z-30 flex h-6 w-6 items-center justify-center rounded-full border border-zinc-200 bg-white text-zinc-500 shadow-sm transition-[left] duration-200 ease-in-out hover:bg-zinc-100 hover:text-zinc-900',
                sidebarCollapsed ? 'left-1.5' : 'left-[218px]',
              )}
              style={{ top: 'calc(env(safe-area-inset-top, 0px) + 1rem)' }}
            >
              {sidebarCollapsed ? (
                <ChevronRight className="h-3.5 w-3.5" />
              ) : (
                <ChevronLeft className="h-3.5 w-3.5" />
              )}
            </button>
          </>
        )}
        <main className={clsx(
          'flex-1',
          isChat ? 'overflow-hidden' : 'overflow-auto p-6',
          isMobile && !hideBottomNav && !isChat && 'pb-20',
        )}>
          <Suspense fallback={<PageLoader />}>
            <Routes>
              <Route path="/" element={<DashboardPage />} />
              <Route path="/chat/*" element={<ChatPage />} />
              <Route path="/tasks" element={<TasksPage />} />
              <Route path="/tasks/:id" element={<TaskDetailPage />} />
              <Route path="/cron-jobs" element={<CronJobsPage />} />
              <Route path="/schedules" element={<SchedulesPage />} />
              <Route path="/projects" element={<Navigate to="/settings?tab=projects" replace />} />
              <Route path="/projects/:id" element={<ProjectDetailPage />} />
              <Route path="/cost" element={<Navigate to="/" replace />} />
              <Route path="/box" element={<BoxPage />} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="/help" element={<HelpPage />} />
            </Routes>
          </Suspense>
        </main>
        {isMobile && !hideBottomNav && <BottomNav />}
        <OfflineIndicator />
        <UpdateBanner />
        <SearchOverlay open={searchOpen} onClose={closeSearch} initialQuery={searchInitialQuery} />
      </div>
    </SSEProvider>
  )
}

export default function App() {
  return (
    <ErrorBoundary>
      <AuthProvider>
        <Suspense fallback={<FullPageLoader />}>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route
              path="/*"
              element={
                <ProtectedRoute>
                  <AuthenticatedApp />
                </ProtectedRoute>
              }
            />
          </Routes>
        </Suspense>
      </AuthProvider>
    </ErrorBoundary>
  )
}
