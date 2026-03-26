import { useRegisterSW } from 'virtual:pwa-register/react'
import { RefreshCw, X } from 'lucide-react'

export default function UpdateBanner() {
  const {
    needRefresh: [needRefresh, setNeedRefresh],
    updateServiceWorker,
  } = useRegisterSW()

  if (!needRefresh) return null

  return (
    <div className="fixed bottom-4 left-1/2 z-50 -translate-x-1/2 flex items-center gap-3 rounded-lg border border-indigo-200 bg-indigo-50 px-4 py-2.5 shadow-lg">
      <RefreshCw className="h-4 w-4 flex-shrink-0 text-indigo-600" />
      <span className="text-sm font-medium text-indigo-900">New version available</span>
      <button
        onClick={() => updateServiceWorker(true)}
        className="rounded-md bg-indigo-600 px-3 py-1 text-xs font-semibold text-white hover:bg-indigo-700 transition-colors"
      >
        Refresh
      </button>
      <button
        onClick={() => setNeedRefresh(false)}
        className="text-indigo-400 hover:text-indigo-600 transition-colors"
        aria-label="Dismiss"
      >
        <X className="h-4 w-4" />
      </button>
    </div>
  )
}
