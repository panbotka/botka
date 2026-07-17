import { useState, useEffect, useRef } from 'react'
import { Plus, Globe, Trash2 } from 'lucide-react'
import type { Bookmark } from '../types'
import { fetchBookmarks, createBookmark, deleteBookmark } from '../api/client'

interface Props {
  /**
   * `inline` renders the bar as a compact row of favicons meant to sit inside the
   * chat header (desktop). `row` renders a full-width, horizontally scrollable
   * second row beneath the header (mobile, where the header row is already full).
   */
  variant: 'inline' | 'row'
  /** Read-only users can open bookmarks but cannot add or delete them. */
  readOnly?: boolean
}

/**
 * BookmarkFavicon renders a bookmark's favicon, falling back to a generic globe
 * icon when the image is missing or fails to load.
 */
function BookmarkFavicon({ bookmark }: { bookmark: Bookmark }) {
  const [broken, setBroken] = useState(false)
  if (broken || !bookmark.favicon_url) {
    return <Globe className="w-4 h-4 text-zinc-400" />
  }
  return (
    <img
      src={bookmark.favicon_url}
      alt=""
      loading="lazy"
      onError={() => setBroken(true)}
      className="w-4 h-4 rounded-sm object-contain"
    />
  )
}

/**
 * BookmarksBar shows the app-wide bookmarks (favicon-only, opening in a new tab)
 * with an inline add popover and a right-click delete menu. Bookmarks are global,
 * so the same list is loaded regardless of the active thread.
 */
export default function BookmarksBar({ variant, readOnly = false }: Props) {
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([])
  const [adding, setAdding] = useState(false)
  const [newUrl, setNewUrl] = useState('')
  const [creating, setCreating] = useState(false)
  const [menu, setMenu] = useState<{ id: number; x: number; y: number } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    let active = true
    fetchBookmarks()
      .then((bs) => { if (active) setBookmarks(bs) })
      .catch(() => { /* ignore — bar just stays empty */ })
    return () => { active = false }
  }, [])

  useEffect(() => {
    if (adding) inputRef.current?.focus()
  }, [adding])

  const flashError = (msg: string) => {
    setError(msg)
    setTimeout(() => setError(null), 3000)
  }

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault()
    const url = newUrl.trim()
    if (!url || creating) return
    setCreating(true)
    try {
      const created = await createBookmark(url)
      setBookmarks((prev) => [...prev, created])
      setNewUrl('')
      setAdding(false)
    } catch {
      flashError('Failed to add bookmark')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: number) => {
    setMenu(null)
    const previous = bookmarks
    setBookmarks((prev) => prev.filter((b) => b.id !== id))
    try {
      await deleteBookmark(id)
    } catch {
      setBookmarks(previous)
      flashError('Failed to delete bookmark')
    }
  }

  const openMenu = (e: React.MouseEvent, id: number) => {
    e.preventDefault()
    setMenu({ id, x: e.clientX, y: e.clientY })
  }

  // Hide the whole bar when there is nothing to show and nothing can be added.
  if (readOnly && bookmarks.length === 0) return null

  const outerClass =
    variant === 'row'
      ? 'flex items-center gap-1 px-4 h-10 bg-zinc-50 border-b border-zinc-200 flex-shrink-0'
      : 'flex items-center gap-0.5 flex-shrink-0'
  const listClass =
    variant === 'row'
      ? 'flex items-center gap-1 overflow-x-auto min-w-0'
      : 'flex items-center gap-0.5'

  return (
    <>
      <div className={outerClass} data-testid="bookmarks-bar">
        <div className={listClass}>
          {bookmarks.map((b) => (
            <a
              key={b.id}
              href={b.url}
              target="_blank"
              rel="noopener noreferrer"
              title={b.title || b.url}
              onContextMenu={readOnly ? undefined : (e) => openMenu(e, b.id)}
              className="flex items-center justify-center w-7 h-7 rounded-md
                         hover:bg-zinc-200/70 transition-colors flex-shrink-0"
            >
              <BookmarkFavicon bookmark={b} />
            </a>
          ))}
        </div>

        {!readOnly && (
          <div className="relative flex-shrink-0">
            <button
              onClick={() => setAdding((a) => !a)}
              title="Add bookmark"
              aria-label="Add bookmark"
              className="flex items-center justify-center w-7 h-7 rounded-md text-zinc-400
                         hover:text-zinc-700 hover:bg-zinc-200/70 transition-colors cursor-pointer"
            >
              <Plus className="w-4 h-4" />
            </button>
            {adding && (
              <>
                <div className="fixed inset-0 z-40" onClick={() => setAdding(false)} />
                <form
                  onSubmit={handleCreate}
                  className="absolute top-full right-0 mt-2 z-50 flex items-center gap-1
                             bg-white dark:bg-zinc-100 border border-zinc-200 rounded-lg shadow-lg p-1.5"
                >
                  <input
                    ref={inputRef}
                    type="text"
                    value={newUrl}
                    onChange={(e) => setNewUrl(e.target.value)}
                    onKeyDown={(e) => { if (e.key === 'Escape') setAdding(false) }}
                    placeholder="https://..."
                    className="w-48 text-xs bg-transparent px-2 py-1 text-zinc-700
                               placeholder-zinc-300 focus:outline-none"
                  />
                  <button
                    type="submit"
                    disabled={creating || !newUrl.trim()}
                    className="text-xs font-medium px-2 py-1 rounded-md bg-zinc-900 text-white
                               hover:bg-zinc-800 disabled:opacity-40 disabled:cursor-not-allowed
                               transition-colors cursor-pointer"
                  >
                    {creating ? 'Adding…' : 'Add'}
                  </button>
                </form>
              </>
            )}
          </div>
        )}
      </div>

      {menu && (
        <>
          <div
            className="fixed inset-0 z-40"
            onClick={() => setMenu(null)}
            onContextMenu={(e) => { e.preventDefault(); setMenu(null) }}
          />
          <div
            className="fixed z-50 bg-white dark:bg-zinc-100 border border-zinc-200 rounded-lg shadow-lg py-1"
            style={{ top: menu.y, left: menu.x }}
          >
            <button
              onClick={() => handleDelete(menu.id)}
              className="flex items-center gap-2 w-full px-3 py-1.5 text-xs text-red-600
                         hover:bg-red-50 transition-colors cursor-pointer"
            >
              <Trash2 className="w-3.5 h-3.5" />
              Delete
            </button>
          </div>
        </>
      )}

      {error && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 z-50 animate-message-in">
          <div className="bg-zinc-800 text-white text-xs px-3 py-2 rounded-lg shadow-lg">
            {error}
          </div>
        </div>
      )}
    </>
  )
}
