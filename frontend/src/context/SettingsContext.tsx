import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react'

export type Theme = 'light' | 'dark' | 'dark-green' | 'dark-blue' | 'system'
export type FontSize = 'small' | 'medium' | 'large'

export interface Settings {
  theme: Theme
  fontSize: FontSize
  sendOnEnter: boolean
  notificationsEnabled: boolean
  notificationSound: boolean
}

const DEFAULT_SETTINGS: Settings = {
  theme: 'light',
  fontSize: 'medium',
  sendOnEnter: true,
  notificationsEnabled: false,
  notificationSound: false,
}

const STORAGE_KEY = 'botka-settings'

interface SettingsContextType {
  settings: Settings
  updateSettings: (partial: Partial<Settings>) => void
  resetSettings: () => void
  resolvedTheme: 'light' | 'dark' | 'dark-green' | 'dark-blue'
}

const SettingsContext = createContext<SettingsContextType | null>(null)

function loadSettings(): Settings {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) {
      return { ...DEFAULT_SETTINGS, ...JSON.parse(stored) }
    }
  } catch { /* ignore */ }
  return { ...DEFAULT_SETTINGS }
}

function saveSettings(settings: Settings) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(settings))
  } catch { /* ignore */ }
}

function getSystemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function SettingsProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<Settings>(loadSettings)
  const [systemTheme, setSystemTheme] = useState<'light' | 'dark'>(getSystemTheme)

  const resolvedTheme: 'light' | 'dark' | 'dark-green' | 'dark-blue' =
    settings.theme === 'system' ? systemTheme : settings.theme

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = (e: MediaQueryListEvent) => setSystemTheme(e.matches ? 'dark' : 'light')
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [])

  useEffect(() => {
    const root = document.documentElement
    root.classList.remove('light', 'dark', 'dark-green', 'dark-blue')
    root.classList.add(resolvedTheme)
  }, [resolvedTheme])

  useEffect(() => {
    const root = document.documentElement
    root.classList.remove('font-small', 'font-medium', 'font-large')
    root.classList.add(`font-${settings.fontSize}`)
  }, [settings.fontSize])

  const updateSettings = useCallback((partial: Partial<Settings>) => {
    setSettings((prev) => {
      const next = { ...prev, ...partial }
      saveSettings(next)
      return next
    })
  }, [])

  const resetSettings = useCallback(() => {
    setSettings({ ...DEFAULT_SETTINGS })
    saveSettings({ ...DEFAULT_SETTINGS })
  }, [])

  return (
    <SettingsContext.Provider value={{ settings, updateSettings, resetSettings, resolvedTheme }}>
      {children}
    </SettingsContext.Provider>
  )
}

export function useSettings() {
  const ctx = useContext(SettingsContext)
  if (!ctx) throw new Error('useSettings must be used within SettingsProvider')
  return ctx
}
