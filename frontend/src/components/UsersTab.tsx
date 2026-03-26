import { useState, useEffect, useCallback } from 'react'
import { clsx } from 'clsx'
import { Loader2, Plus, Trash2, X, Key, Link2, Unlink } from 'lucide-react'
import {
  fetchUsers,
  createUser,
  deleteUser,
  resetUserPassword,
  fetchUserThreads,
  grantUserThread,
  revokeUserThread,
  fetchThreads,
  type ExternalUser,
  type UserThreadAccess,
} from '../api/client'
import type { Thread } from '../types'

export default function UsersTab() {
  const [users, setUsers] = useState<ExternalUser[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // Create user form
  const [showCreate, setShowCreate] = useState(false)
  const [newUsername, setNewUsername] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [creating, setCreating] = useState(false)

  // Password reset
  const [resetUserId, setResetUserId] = useState<number | null>(null)
  const [resetPass, setResetPass] = useState('')
  const [resetting, setResetting] = useState(false)

  // Thread access management
  const [accessUserId, setAccessUserId] = useState<number | null>(null)
  const [userThreads, setUserThreads] = useState<UserThreadAccess[]>([])
  const [allThreads, setAllThreads] = useState<Thread[]>([])
  const [loadingAccess, setLoadingAccess] = useState(false)
  const [grantThreadId, setGrantThreadId] = useState<number | ''>('')

  const loadUsers = useCallback(async () => {
    try {
      const data = await fetchUsers()
      setUsers(data)
    } catch {
      setError('Failed to load users')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { loadUsers() }, [loadUsers])

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault()
    setCreating(true)
    setError('')
    try {
      await createUser(newUsername, newPassword)
      setNewUsername('')
      setNewPassword('')
      setShowCreate(false)
      await loadUsers()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create user')
    } finally {
      setCreating(false)
    }
  }

  async function handleDelete(id: number, username: string) {
    if (!confirm(`Delete user "${username}"? This will revoke all their thread access.`)) return
    try {
      await deleteUser(id)
      if (accessUserId === id) setAccessUserId(null)
      await loadUsers()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete user')
    }
  }

  async function handleResetPassword(e: React.FormEvent) {
    e.preventDefault()
    if (resetUserId === null) return
    setResetting(true)
    setError('')
    try {
      await resetUserPassword(resetUserId, resetPass)
      setResetUserId(null)
      setResetPass('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reset password')
    } finally {
      setResetting(false)
    }
  }

  async function openAccessManager(userId: number) {
    setAccessUserId(userId)
    setLoadingAccess(true)
    try {
      const [threads, allT] = await Promise.all([
        fetchUserThreads(userId),
        fetchThreads(),
      ])
      setUserThreads(threads)
      setAllThreads(allT)
    } catch {
      setError('Failed to load thread access')
    } finally {
      setLoadingAccess(false)
    }
  }

  async function handleGrant() {
    if (accessUserId === null || grantThreadId === '') return
    try {
      await grantUserThread(accessUserId, Number(grantThreadId))
      setGrantThreadId('')
      // Reload
      const threads = await fetchUserThreads(accessUserId)
      setUserThreads(threads)
      await loadUsers()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to grant access')
    }
  }

  async function handleRevoke(threadId: number) {
    if (accessUserId === null) return
    try {
      await revokeUserThread(accessUserId, threadId)
      const threads = await fetchUserThreads(accessUserId)
      setUserThreads(threads)
      await loadUsers()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to revoke access')
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
      </div>
    )
  }

  const externalUsers = users.filter((u) => u.role === 'external')
  const assignedThreadIds = new Set(userThreads.map((t) => t.thread_id))
  const availableThreads = allThreads.filter((t) => !assignedThreadIds.has(t.id))

  return (
    <div className="space-y-6">
      {error && (
        <div className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700 flex items-center justify-between">
          {error}
          <button onClick={() => setError('')} className="text-red-400 hover:text-red-600">
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      {/* Header */}
      <div className="flex items-center justify-between">
        <p className="text-sm text-zinc-500">
          External users can only access threads you assign to them.
        </p>
        <button
          onClick={() => setShowCreate(!showCreate)}
          className="flex items-center gap-1 rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-zinc-50 hover:bg-zinc-800"
        >
          <Plus className="h-4 w-4" />
          Add User
        </button>
      </div>

      {/* Create user form */}
      {showCreate && (
        <form onSubmit={handleCreate} className="rounded-lg border border-zinc-200 bg-zinc-50 p-4 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-zinc-600 mb-1">Username</label>
              <input
                type="text"
                value={newUsername}
                onChange={(e) => setNewUsername(e.target.value)}
                className="w-full rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm"
                required
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-600 mb-1">Password</label>
              <input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                className="w-full rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm"
                required
                minLength={8}
              />
            </div>
          </div>
          <div className="flex gap-2">
            <button
              type="submit"
              disabled={creating}
              className={clsx(
                'rounded-md px-3 py-1.5 text-sm font-medium text-zinc-50',
                creating ? 'bg-zinc-400' : 'bg-zinc-900 hover:bg-zinc-800',
              )}
            >
              {creating ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Create'}
            </button>
            <button
              type="button"
              onClick={() => { setShowCreate(false); setNewUsername(''); setNewPassword('') }}
              className="rounded-md px-3 py-1.5 text-sm text-zinc-500 hover:text-zinc-700"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      {/* User list */}
      {externalUsers.length === 0 ? (
        <p className="py-8 text-center text-sm text-zinc-400">No external users yet.</p>
      ) : (
        <div className="divide-y divide-zinc-200 rounded-lg border border-zinc-200">
          {externalUsers.map((u) => (
            <div key={u.id} className="flex items-center justify-between px-4 py-3">
              <div>
                <span className="text-sm font-medium text-zinc-900">{u.username}</span>
                <span className="ml-2 text-xs text-zinc-400">
                  {u.thread_count} thread{u.thread_count !== 1 ? 's' : ''}
                </span>
              </div>
              <div className="flex items-center gap-1">
                <button
                  onClick={() => openAccessManager(u.id)}
                  title="Manage thread access"
                  className={clsx(
                    'rounded-md p-1.5 text-zinc-400 hover:text-zinc-700 hover:bg-zinc-100',
                    accessUserId === u.id && 'bg-zinc-100 text-zinc-700',
                  )}
                >
                  <Link2 className="h-4 w-4" />
                </button>
                <button
                  onClick={() => setResetUserId(resetUserId === u.id ? null : u.id)}
                  title="Reset password"
                  className={clsx(
                    'rounded-md p-1.5 text-zinc-400 hover:text-zinc-700 hover:bg-zinc-100',
                    resetUserId === u.id && 'bg-zinc-100 text-zinc-700',
                  )}
                >
                  <Key className="h-4 w-4" />
                </button>
                <button
                  onClick={() => handleDelete(u.id, u.username)}
                  title="Delete user"
                  className="rounded-md p-1.5 text-zinc-400 hover:text-red-600 hover:bg-red-50"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Password reset inline form */}
      {resetUserId !== null && (
        <form onSubmit={handleResetPassword} className="rounded-lg border border-zinc-200 bg-zinc-50 p-4 space-y-3">
          <p className="text-sm font-medium text-zinc-700">
            Reset password for {users.find((u) => u.id === resetUserId)?.username}
          </p>
          <div className="flex gap-2">
            <input
              type="password"
              value={resetPass}
              onChange={(e) => setResetPass(e.target.value)}
              placeholder="New password (min 8 chars)"
              className="flex-1 rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm"
              required
              minLength={8}
            />
            <button
              type="submit"
              disabled={resetting}
              className={clsx(
                'rounded-md px-3 py-1.5 text-sm font-medium text-zinc-50',
                resetting ? 'bg-zinc-400' : 'bg-zinc-900 hover:bg-zinc-800',
              )}
            >
              {resetting ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Reset'}
            </button>
            <button
              type="button"
              onClick={() => { setResetUserId(null); setResetPass('') }}
              className="rounded-md px-3 py-1.5 text-sm text-zinc-500 hover:text-zinc-700"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      {/* Thread access manager */}
      {accessUserId !== null && (
        <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-4 space-y-3">
          <div className="flex items-center justify-between">
            <p className="text-sm font-medium text-zinc-700">
              Thread access for {users.find((u) => u.id === accessUserId)?.username}
            </p>
            <button
              onClick={() => setAccessUserId(null)}
              className="text-zinc-400 hover:text-zinc-600"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          {loadingAccess ? (
            <div className="flex justify-center py-4">
              <Loader2 className="h-5 w-5 animate-spin text-zinc-400" />
            </div>
          ) : (
            <>
              {/* Grant new access */}
              <div className="flex gap-2">
                <select
                  value={grantThreadId}
                  onChange={(e) => setGrantThreadId(e.target.value ? Number(e.target.value) : '')}
                  className="flex-1 rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm"
                >
                  <option value="">Select a thread to grant access...</option>
                  {availableThreads.map((t) => (
                    <option key={t.id} value={t.id}>{t.title}</option>
                  ))}
                </select>
                <button
                  onClick={handleGrant}
                  disabled={grantThreadId === ''}
                  className={clsx(
                    'flex items-center gap-1 rounded-md px-3 py-1.5 text-sm font-medium text-zinc-50',
                    grantThreadId === '' ? 'bg-zinc-400' : 'bg-zinc-900 hover:bg-zinc-800',
                  )}
                >
                  <Plus className="h-4 w-4" />
                  Grant
                </button>
              </div>

              {/* Current access list */}
              {userThreads.length === 0 ? (
                <p className="text-sm text-zinc-400">No threads assigned yet.</p>
              ) : (
                <div className="divide-y divide-zinc-200 rounded-md border border-zinc-200 bg-white">
                  {userThreads.map((ta) => (
                    <div key={ta.thread_id} className="flex items-center justify-between px-3 py-2">
                      <span className="text-sm text-zinc-700">{ta.thread_title}</span>
                      <button
                        onClick={() => handleRevoke(ta.thread_id)}
                        title="Revoke access"
                        className="rounded p-1 text-zinc-400 hover:text-red-600 hover:bg-red-50"
                      >
                        <Unlink className="h-4 w-4" />
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}
