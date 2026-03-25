import { lazy, Suspense } from 'react'
import { Routes, Route, NavLink, useLocation } from 'react-router-dom'
import {
  LayoutDashboard,
  MessageSquare,
  ListTodo,
  FolderGit2,
  Settings,
  HelpCircle,
  Loader2,
} from 'lucide-react'
import { clsx } from 'clsx'
import { useIsMobile } from './hooks/useIsMobile'
import BottomNav from './components/BottomNav'
import OfflineIndicator from './components/OfflineIndicator'

const DashboardPage = lazy(() => import('./pages/DashboardPage'))
const ChatPage = lazy(() => import('./pages/ChatPage'))
const TasksPage = lazy(() => import('./pages/TasksPage'))
const TaskDetailPage = lazy(() => import('./pages/TaskDetailPage'))
const ProjectsPage = lazy(() => import('./pages/ProjectsPage'))
const SettingsPage = lazy(() => import('./pages/SettingsPage'))
const HelpPage = lazy(() => import('./pages/HelpPage'))

const navItems = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/chat', icon: MessageSquare, label: 'Chat' },
  { to: '/tasks', icon: ListTodo, label: 'Tasks' },
  { to: '/projects', icon: FolderGit2, label: 'Projects' },
  { to: '/settings', icon: Settings, label: 'Settings' },
  { to: '/help', icon: HelpCircle, label: 'Help' },
] as const

function PageLoader() {
  return (
    <div className="flex h-64 items-center justify-center">
      <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
    </div>
  )
}

function AppSidebar() {
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
                  ? 'bg-zinc-200 text-zinc-900'
                  : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900',
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

export default function App() {
  const location = useLocation()
  const isMobile = useIsMobile()
  const isChat = location.pathname.startsWith('/chat')
  // Hide bottom nav when viewing an active chat thread on mobile
  const isActiveChatThread = /^\/chat\/\d+$/.test(location.pathname)
  const hideBottomNav = isMobile && isActiveChatThread

  return (
    <div className="flex h-screen bg-zinc-50">
      {!isMobile && <AppSidebar />}
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
            <Route path="/projects" element={<ProjectsPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/help" element={<HelpPage />} />
          </Routes>
        </Suspense>
      </main>
      {isMobile && !hideBottomNav && <BottomNav />}
      <OfflineIndicator />
    </div>
  )
}
