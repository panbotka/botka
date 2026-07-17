import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import BookmarksBar from './BookmarksBar'
import type { Bookmark } from '../types'

const fetchBookmarks = vi.fn()
const createBookmark = vi.fn()
const deleteBookmark = vi.fn()

vi.mock('../api/client', () => ({
  fetchBookmarks: (...args: unknown[]) => fetchBookmarks(...args),
  createBookmark: (...args: unknown[]) => createBookmark(...args),
  deleteBookmark: (...args: unknown[]) => deleteBookmark(...args),
}))

function bookmark(overrides: Partial<Bookmark> = {}): Bookmark {
  return {
    id: 1,
    url: 'https://example.com',
    title: 'Example',
    favicon_url: 'https://example.com/favicon.ico',
    sort_order: 0,
    created_at: '',
    updated_at: '',
    ...overrides,
  }
}

describe('BookmarksBar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders bookmarks as favicon links that open in a new tab', async () => {
    fetchBookmarks.mockResolvedValue([bookmark({ title: 'My Site', url: 'https://my.site' })])

    render(<BookmarksBar variant="inline" />)

    const link = await screen.findByTitle('My Site')
    expect(link.getAttribute('href')).toBe('https://my.site')
    expect(link.getAttribute('target')).toBe('_blank')
    expect(link.getAttribute('rel')).toBe('noopener noreferrer')
    // Favicon rendered as an <img>.
    expect(link.querySelector('img')?.getAttribute('src')).toBe('https://example.com/favicon.ico')
  })

  it('falls back to a globe icon when the favicon fails to load', async () => {
    fetchBookmarks.mockResolvedValue([bookmark()])

    render(<BookmarksBar variant="inline" />)

    const img = (await screen.findByTitle('Example')).querySelector('img')!
    fireEvent.error(img)

    await waitFor(() => {
      const link = screen.getByTitle('Example')
      expect(link.querySelector('img')).toBeNull()
      expect(link.querySelector('svg')).not.toBeNull()
    })
  })

  it('adds a bookmark through the inline input', async () => {
    fetchBookmarks.mockResolvedValue([])
    createBookmark.mockResolvedValue(bookmark({ id: 5, title: 'New', url: 'https://new.site' }))

    render(<BookmarksBar variant="inline" />)

    fireEvent.click(await screen.findByLabelText('Add bookmark'))
    fireEvent.change(screen.getByPlaceholderText('https://...'), { target: { value: 'https://new.site' } })
    fireEvent.click(screen.getByText('Add'))

    await waitFor(() => expect(createBookmark).toHaveBeenCalledWith('https://new.site'))
    expect(await screen.findByTitle('New')).toBeTruthy()
  })

  it('deletes a bookmark via the right-click menu', async () => {
    fetchBookmarks.mockResolvedValue([bookmark({ id: 7, title: 'Doomed' })])
    deleteBookmark.mockResolvedValue(undefined)

    render(<BookmarksBar variant="inline" />)

    const link = await screen.findByTitle('Doomed')
    fireEvent.contextMenu(link)
    fireEvent.click(await screen.findByText('Delete'))

    await waitFor(() => expect(deleteBookmark).toHaveBeenCalledWith(7))
    expect(screen.queryByTitle('Doomed')).toBeNull()
  })

  it('restores the bookmark when deletion fails', async () => {
    fetchBookmarks.mockResolvedValue([bookmark({ id: 7, title: 'Doomed' })])
    deleteBookmark.mockRejectedValue(new Error('boom'))

    render(<BookmarksBar variant="inline" />)

    fireEvent.contextMenu(await screen.findByTitle('Doomed'))
    fireEvent.click(await screen.findByText('Delete'))

    expect(await screen.findByText('Failed to delete bookmark')).toBeTruthy()
    expect(screen.getByTitle('Doomed')).toBeTruthy()
  })

  it('hides add/delete affordances for read-only users', async () => {
    fetchBookmarks.mockResolvedValue([bookmark({ title: 'Read Only' })])

    render(<BookmarksBar variant="inline" readOnly />)

    expect(await screen.findByTitle('Read Only')).toBeTruthy()
    expect(screen.queryByLabelText('Add bookmark')).toBeNull()
    // Right-clicking does not open a delete menu.
    fireEvent.contextMenu(screen.getByTitle('Read Only'))
    expect(screen.queryByText('Delete')).toBeNull()
  })

  it('renders nothing for read-only users with no bookmarks', async () => {
    fetchBookmarks.mockResolvedValue([])

    const { container } = render(<BookmarksBar variant="row" readOnly />)

    await waitFor(() => expect(fetchBookmarks).toHaveBeenCalled())
    expect(container.querySelector('[data-testid="bookmarks-bar"]')).toBeNull()
  })
})
