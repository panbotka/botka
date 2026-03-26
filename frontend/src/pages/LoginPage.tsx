import { useState, useCallback, useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { Loader2, KeyRound, LogIn } from 'lucide-react'
import { clsx } from 'clsx'
import { useAuth } from '../context/AuthContext'
import { passkeyLoginBegin, passkeyLoginFinish } from '../api/client'

function base64urlToBuffer(base64url: string): ArrayBuffer {
  const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/')
  const pad = base64.length % 4
  const padded = pad ? base64 + '='.repeat(4 - pad) : base64
  const binary = atob(padded)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes.buffer
}

export default function LoginPage() {
  const { login, isAuthenticated, user } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [passkeyLoading, setPasskeyLoading] = useState(false)
  const [supportsPasskey, setSupportsPasskey] = useState(false)

  const defaultRedirect = user?.role === 'external' ? '/chat' : '/'
  const redirectTo = (location.state as { from?: string })?.from || defaultRedirect

  useEffect(() => {
    if (isAuthenticated) {
      // External users should always go to /chat, even if they had a saved redirect.
      const target = user?.role === 'external' ? '/chat' : redirectTo
      navigate(target, { replace: true })
    }
  }, [isAuthenticated, navigate, redirectTo, user?.role])

  useEffect(() => {
    // Check if WebAuthn is available.
    if (window.PublicKeyCredential) {
      PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable?.()
        .then((available) => setSupportsPasskey(available))
        .catch(() => setSupportsPasskey(false))
    }
  }, [])

  const handlePasswordLogin = useCallback(async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await login(username, password)
      navigate(redirectTo, { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    } finally {
      setLoading(false)
    }
  }, [username, password, login, navigate, redirectTo])

  const handlePasskeyLogin = useCallback(async () => {
    setError('')
    setPasskeyLoading(true)
    try {
      const beginRes = await passkeyLoginBegin()
      const options = beginRes.data

      // Convert base64url fields to ArrayBuffers for the WebAuthn API.
      const publicKeyOptions: PublicKeyCredentialRequestOptions = {
        ...options,
        challenge: base64urlToBuffer(options.challenge as unknown as string),
        allowCredentials: (options.allowCredentials as unknown as Array<{ id: string; type: string }>)?.map(
          (cred) => ({
            id: base64urlToBuffer(cred.id),
            type: 'public-key' as const,
          }),
        ),
      }

      const credential = await navigator.credentials.get({ publicKey: publicKeyOptions })
      if (!credential) {
        setError('Passkey authentication was cancelled')
        return
      }

      await passkeyLoginFinish(credential)
      navigate(redirectTo, { replace: true })
      // Force refresh to pick up the user state
      window.location.reload()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Passkey login failed')
    } finally {
      setPasskeyLoading(false)
    }
  }, [navigate, redirectTo])

  return (
    <div className="flex min-h-dvh items-center justify-center bg-zinc-100 p-4">
      <div className="w-full max-w-sm space-y-6 rounded-xl border border-zinc-200 bg-white dark:bg-zinc-100 p-8 shadow-sm">
        <div className="text-center">
          <span className="text-4xl">🤖</span>
          <h1 className="mt-2 text-xl font-semibold text-zinc-900">Botka</h1>
          <p className="mt-1 text-sm text-zinc-500">Sign in to continue</p>
        </div>

        {error && (
          <div className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">
            {error}
          </div>
        )}

        <form onSubmit={handlePasswordLogin} className="space-y-4">
          <div>
            <label htmlFor="username" className="block text-sm font-medium text-zinc-700">
              Username
            </label>
            <input
              id="username"
              type="text"
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="mt-1 block w-full rounded-md border border-zinc-300 bg-zinc-50 px-3 py-2 text-sm text-zinc-900 shadow-sm focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
              required
            />
          </div>
          <div>
            <label htmlFor="password" className="block text-sm font-medium text-zinc-700">
              Password
            </label>
            <input
              id="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="mt-1 block w-full rounded-md border border-zinc-300 bg-zinc-50 px-3 py-2 text-sm text-zinc-900 shadow-sm focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
              required
            />
          </div>
          <button
            type="submit"
            disabled={loading}
            className={clsx(
              'flex w-full items-center justify-center gap-2 rounded-md px-4 py-2 text-sm font-medium text-zinc-50 transition-colors',
              loading
                ? 'cursor-not-allowed bg-zinc-400'
                : 'bg-zinc-900 hover:bg-zinc-800',
            )}
          >
            {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <LogIn className="h-4 w-4" />}
            Sign in
          </button>
        </form>

        {supportsPasskey && (
          <>
            <div className="relative">
              <div className="absolute inset-0 flex items-center">
                <div className="w-full border-t border-zinc-200" />
              </div>
              <div className="relative flex justify-center text-xs">
                <span className="bg-white dark:bg-zinc-100 px-2 text-zinc-400">or</span>
              </div>
            </div>
            <button
              onClick={handlePasskeyLogin}
              disabled={passkeyLoading}
              className={clsx(
                'flex w-full items-center justify-center gap-2 rounded-md border px-4 py-2 text-sm font-medium transition-colors',
                passkeyLoading
                  ? 'cursor-not-allowed border-zinc-200 bg-zinc-50 text-zinc-400'
                  : 'border-zinc-300 bg-white dark:bg-zinc-100 text-zinc-700 hover:bg-zinc-50',
              )}
            >
              {passkeyLoading ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <KeyRound className="h-4 w-4" />
              )}
              Sign in with passkey
            </button>
          </>
        )}
      </div>
    </div>
  )
}
