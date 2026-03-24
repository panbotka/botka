import { Routes, Route, NavLink } from 'react-router-dom'
import {
  LayoutDashboard,
  MessageSquare,
  ListTodo,
  FolderGit2,
  Settings,
} from 'lucide-react'
import { clsx } from 'clsx'

const navItems = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/chat', icon: MessageSquare, label: 'Chat' },
  { to: '/tasks', icon: ListTodo, label: 'Tasks' },
  { to: '/projects', icon: FolderGit2, label: 'Projects' },
  { to: '/settings', icon: Settings, label: 'Settings' },
] as const

function Placeholder({ name }: { name: string }) {
  return (
    <div className="flex items-center justify-center h-full text-zinc-400 text-lg">
      {name} — Coming Soon
    </div>
  )
}

function Sidebar() {
  return (
    <aside className="flex h-screen w-56 flex-col border-r border-zinc-200 bg-zinc-50">
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
  return (
    <div className="flex h-screen bg-white">
      <Sidebar />
      <main className="flex-1 overflow-auto p-6">
        <Routes>
          <Route path="/" element={<Placeholder name="Dashboard" />} />
          <Route path="/chat" element={<Placeholder name="Chat" />} />
          <Route path="/chat/:id" element={<Placeholder name="Chat" />} />
          <Route path="/tasks" element={<Placeholder name="Tasks" />} />
          <Route path="/tasks/:id" element={<Placeholder name="Task Detail" />} />
          <Route path="/projects" element={<Placeholder name="Projects" />} />
          <Route path="/settings" element={<Placeholder name="Settings" />} />
        </Routes>
      </main>
    </div>
  )
}
