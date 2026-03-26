import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react'
import { authMe, authLogin, authLogout, setOnUnauthorized, type AuthUser } from '../api/client'

interface AuthContextValue {
  isAuthenticated: boolean
  isLoading: boolean
  user: AuthUser | null
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  refreshUser: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  const clearAuth = useCallback(() => {
    setUser(null)
  }, [])

  // Register 401 handler.
  useEffect(() => {
    setOnUnauthorized(clearAuth)
  }, [clearAuth])

  const refreshUser = useCallback(async () => {
    try {
      const u = await authMe()
      setUser(u)
    } catch {
      setUser(null)
    }
  }, [])

  // Check auth on mount.
  useEffect(() => {
    refreshUser().finally(() => setIsLoading(false))
  }, [refreshUser])

  const login = useCallback(async (username: string, password: string) => {
    const u = await authLogin(username, password)
    setUser(u)
  }, [])

  const logout = useCallback(async () => {
    await authLogout()
    setUser(null)
  }, [])

  return (
    <AuthContext.Provider
      value={{
        isAuthenticated: user !== null,
        isLoading,
        user,
        login,
        logout,
        refreshUser,
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
