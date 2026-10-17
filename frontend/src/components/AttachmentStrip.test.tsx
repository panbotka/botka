import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import AttachmentStrip, { attachmentKind } from './AttachmentStrip'
import type { Attachment } from '../types'

function att(id: number, name: string, mime: string, size = 2048): Attachment {
  return {
    id,
    message_id: 1,
    stored_name: `${id}-${name}`,
    original_name: name,
    mime_type: mime,
    size,
    url: `/uploads/${id}-${name}`,
    created_at: new Date().toISOString(),
  }
}

const SIX = [
  att(1, 'a.png', 'image/png'),
  att(2, 'b.jpg', 'image/jpeg'),
  att(3, 'c.webp', 'image/webp'),
  att(4, 'd.gif', 'image/gif'),
  att(5, 'report.pdf', 'application/pdf'),
  att(6, 'data.csv', 'text/csv'),
]

describe('AttachmentStrip', () => {
  it('renders nothing without attachments', () => {
    const { container } = render(<AttachmentStrip attachments={[]} variant="assistant" />)
    expect(container.firstChild).toBeNull()
  })

  it('renders up to 4 attachments expanded, thumbnails before chips', () => {
    const items = [att(1, 'notes.txt', 'text/plain'), att(2, 'a.png', 'image/png')]
    const { container } = render(<AttachmentStrip attachments={items} variant="assistant" />)
    expect(screen.queryByTestId('attachment-toggle')).toBeNull()
    expect(screen.getAllByRole('img', { name: 'a.png' })).toHaveLength(1)
    expect(screen.getAllByTestId('attachment-chip')).toHaveLength(1)
    const thumbs = screen.getByTestId('attachment-thumbnails')
    const chip = screen.getByTestId('attachment-chip')
    // Thumbnails row precedes the chip in document order.
    expect(thumbs.compareDocumentPosition(chip) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(container.textContent).toContain('2.0 KB')
  })

  it('collapses more than 4 attachments into a single row with the count', () => {
    render(<AttachmentStrip attachments={SIX} variant="assistant" />)
    const toggle = screen.getByTestId('attachment-toggle')
    expect(toggle.textContent).toContain('6 souborů')
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(screen.queryAllByTestId('attachment-chip')).toHaveLength(0)
    expect(screen.queryByTestId('attachment-thumbnails')).toBeNull()
    // First three images as tiny previews.
    expect(screen.getAllByTestId('attachment-mini-preview')).toHaveLength(3)
  })

  it('expands to show every attachment and collapses again', () => {
    const onImageClick = vi.fn()
    render(<AttachmentStrip attachments={SIX} variant="user" onImageClick={onImageClick} />)
    const toggle = screen.getByTestId('attachment-toggle')

    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    const thumbs = screen.getByTestId('attachment-thumbnails')
    expect(thumbs.querySelectorAll('button')).toHaveLength(4)
    expect(screen.getAllByTestId('attachment-chip')).toHaveLength(2)
    expect(screen.queryAllByTestId('attachment-mini-preview')).toHaveLength(0)

    fireEvent.click(thumbs.querySelectorAll('button')[1]!)
    expect(onImageClick).toHaveBeenCalledWith(SIX[1], SIX.slice(0, 4))

    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(screen.queryAllByTestId('attachment-chip')).toHaveLength(0)
  })

  it('gives JSON a non-PDF icon', () => {
    const json = att(1, 'payload.json', 'application/json')
    const pdf = att(2, 'doc.pdf', 'application/pdf')
    expect(attachmentKind(json)).toBe('json')
    expect(attachmentKind(pdf)).toBe('pdf')

    render(<AttachmentStrip attachments={[json, pdf]} variant="assistant" />)
    const [jsonChip, pdfChip] = screen.getAllByTestId('attachment-chip') as [HTMLElement, HTMLElement]
    expect(jsonChip.dataset.kind).toBe('json')
    const jsonIcon = jsonChip.querySelector('svg')?.getAttribute('class') ?? ''
    const pdfIcon = pdfChip.querySelector('svg')?.getAttribute('class') ?? ''
    expect(jsonIcon).toContain('lucide-file-json')
    expect(pdfIcon).not.toContain('lucide-file-json')
  })

  it('classifies HTML, CSV and generic files', () => {
    expect(attachmentKind(att(1, 'page.html', 'text/html'))).toBe('html')
    expect(attachmentKind(att(2, 'rows.csv', 'text/csv'))).toBe('csv')
    expect(attachmentKind(att(3, 'main.go', 'text/plain'))).toBe('code')
    expect(attachmentKind(att(4, 'notes.txt', 'text/plain'))).toBe('text')
    expect(attachmentKind(att(5, 'blob.bin', 'application/octet-stream'))).toBe('other')
  })

  it('opens PDFs in a new tab and downloads other files', () => {
    render(
      <AttachmentStrip
        attachments={[att(1, 'doc.pdf', 'application/pdf'), att(2, 'blob.bin', 'application/octet-stream')]}
        variant="assistant"
      />,
    )
    const [pdfChip, binChip] = screen.getAllByTestId('attachment-chip') as [HTMLElement, HTMLElement]
    const pdfOpen = pdfChip.querySelector('a')!
    expect(pdfOpen.getAttribute('target')).toBe('_blank')
    expect(pdfOpen.hasAttribute('download')).toBe(false)
    const binOpen = binChip.querySelector('a')!
    expect(binOpen.getAttribute('download')).toBe('blob.bin')
    // Every chip carries an explicit download link.
    expect(pdfChip.querySelectorAll('a[download]')).toHaveLength(1)
  })
})
