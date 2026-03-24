import { MessageSquare, Search, Settings } from 'lucide-react'
import { clsx } from 'clsx'

export type MobileTab = 'chats' | 'search' | 'settings'

interface Props {
  activeTab: MobileTab
  onTabChange: (tab: MobileTab) => void
}

export default function BottomNav({ activeTab, onTabChange }: Props) {
  const tabs: { id: MobileTab; label: string; icon: React.ReactNode }[] = [
    { id: 'chats', label: 'Chats', icon: <MessageSquare className="w-6 h-6" /> },
    { id: 'search', label: 'Search', icon: <Search className="w-6 h-6" /> },
    { id: 'settings', label: 'Settings', icon: <Settings className="w-6 h-6" /> },
  ]

  return (
    <nav
      className="fixed bottom-0 left-0 right-0 z-40 bg-white border-t border-zinc-200"
      style={{ paddingBottom: 'env(safe-area-inset-bottom, 0px)' }}
    >
      <div className="flex h-14">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => onTabChange(tab.id)}
            className={clsx(
              'flex-1 flex flex-col items-center justify-center gap-0.5 transition-colors cursor-pointer',
              activeTab === tab.id ? 'text-amber-600' : 'text-zinc-400 active:text-zinc-600',
            )}
          >
            {tab.icon}
            <span className="text-[10px] font-medium">{tab.label}</span>
          </button>
        ))}
      </div>
    </nav>
  )
}
