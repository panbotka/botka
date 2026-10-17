import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ModelChip from './ModelChip'
import type { Thread } from '../types'

const updateModel = vi.fn()
const getModels = vi.fn()
vi.mock('../api/client', () => ({
  api: {
    updateModel: (...args: unknown[]) => updateModel(...args),
    getModels: (...args: unknown[]) => getModels(...args),
  },
}))

function thread(overrides: Partial<Thread> = {}): Thread {
  return {
    id: 7,
    title: 'T',
    model: 'sonnet',
    system_prompt: '',
    custom_context: '',
    pinned: false,
    archived: false,
    created_at: '',
    updated_at: '',
    tags: [],
    ...overrides,
  } as Thread
}

beforeEach(() => {
  vi.clearAllMocks()
  getModels.mockResolvedValue({ models: ['sonnet', 'opus', 'haiku'] })
  updateModel.mockResolvedValue(undefined)
})

describe('ModelChip', () => {
  it('shows the current model and lists the available models when tapped', async () => {
    const user = userEvent.setup()
    render(<ModelChip thread={thread()} onChanged={vi.fn()} />)
    expect(screen.getByTestId('model-chip')).toHaveTextContent('sonnet')

    await user.click(screen.getByTestId('model-chip'))
    expect(screen.getByRole('dialog', { name: 'Model' })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: 'opus' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'haiku' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Default (gateway decides)' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'sonnet' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('switches the model immediately on selection', async () => {
    const user = userEvent.setup()
    const onChanged = vi.fn()
    render(<ModelChip thread={thread()} onChanged={onChanged} />)

    await user.click(screen.getByTestId('model-chip'))
    await user.click(await screen.findByRole('button', { name: 'opus' }))

    expect(updateModel).toHaveBeenCalledWith(7, 'opus')
    await waitFor(() => expect(onChanged).toHaveBeenCalled())
    // Sheet closed, chip shows the new model before the thread list reloads.
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByTestId('model-chip')).toHaveTextContent('opus')
  })

  it('reverts the chip when the save fails', async () => {
    updateModel.mockRejectedValue(new Error('nope'))
    const user = userEvent.setup()
    const onChanged = vi.fn()
    render(<ModelChip thread={thread()} onChanged={onChanged} />)

    await user.click(screen.getByTestId('model-chip'))
    await user.click(await screen.findByRole('button', { name: 'haiku' }))

    await waitFor(() => expect(screen.getByTestId('model-chip')).toHaveTextContent('sonnet'))
    expect(onChanged).not.toHaveBeenCalled()
  })

  it('does not save when the current model is picked again', async () => {
    const user = userEvent.setup()
    render(<ModelChip thread={thread()} onChanged={vi.fn()} />)
    await user.click(screen.getByTestId('model-chip'))
    await user.click(await screen.findByRole('button', { name: 'sonnet' }))
    expect(updateModel).not.toHaveBeenCalled()
  })

  it('is a plain label for read-only users', () => {
    render(<ModelChip thread={thread({ model: '' })} onChanged={vi.fn()} readOnly />)
    const chip = screen.getByTestId('model-chip')
    expect(chip.tagName).toBe('SPAN')
    expect(chip).toHaveTextContent('Default')
  })
})
