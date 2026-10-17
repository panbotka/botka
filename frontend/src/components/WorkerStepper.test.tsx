import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import WorkerStepper from './WorkerStepper'
import { MAX_WORKERS, MIN_WORKERS, SAVE_DEBOUNCE_MS, useWorkerCount } from '../hooks/useWorkerCount'

vi.mock('../api/client', () => ({
  updateServerSettings: vi.fn(),
}))

import { updateServerSettings } from '../api/client'

const mockUpdate = vi.mocked(updateServerSettings)

function Harness({ serverValue, onSaved }: { serverValue: number; onSaved?: (n: number) => void }) {
  const w = useWorkerCount(serverValue, onSaved)
  return (
    <div>
      <WorkerStepper
        value={w.value}
        canDecrement={w.canDecrement}
        canIncrement={w.canIncrement}
        onDecrement={w.decrement}
        onIncrement={w.increment}
      />
      {w.error && <p role="alert">{w.error}</p>}
    </div>
  )
}

const count = () => screen.getByTestId('worker-count').textContent
const dec = () => screen.getByRole('button', { name: 'Decrease workers' })
const inc = () => screen.getByRole('button', { name: 'Increase workers' })

async function flushDebounce() {
  await act(async () => {
    vi.advanceTimersByTime(SAVE_DEBOUNCE_MS)
  })
}

describe('WorkerStepper + useWorkerCount', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mockUpdate.mockReset()
    mockUpdate.mockImplementation(async (s) => ({ max_workers: s.max_workers ?? 0 }))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('disables decrement at the minimum', () => {
    render(<Harness serverValue={MIN_WORKERS} />)
    expect(dec()).toBeDisabled()
    expect(inc()).not.toBeDisabled()
    fireEvent.click(dec())
    expect(count()).toBe(String(MIN_WORKERS))
  })

  it('disables increment at the maximum', () => {
    render(<Harness serverValue={MAX_WORKERS} />)
    expect(inc()).toBeDisabled()
    expect(dec()).not.toBeDisabled()
  })

  it('updates optimistically and debounces rapid clicks into one save', async () => {
    const onSaved = vi.fn()
    render(<Harness serverValue={2} onSaved={onSaved} />)

    fireEvent.click(inc())
    fireEvent.click(inc())
    fireEvent.click(inc())
    expect(count()).toBe('5')
    expect(mockUpdate).not.toHaveBeenCalled()

    await act(async () => {
      vi.advanceTimersByTime(SAVE_DEBOUNCE_MS - 1)
    })
    expect(mockUpdate).not.toHaveBeenCalled()

    await flushDebounce()
    expect(mockUpdate).toHaveBeenCalledTimes(1)
    expect(mockUpdate).toHaveBeenCalledWith({ max_workers: 5 })
    expect(onSaved).toHaveBeenCalledWith(5)
    expect(count()).toBe('5')
  })

  it('rolls back and shows an error when the save fails', async () => {
    mockUpdate.mockRejectedValueOnce(new Error('boom'))
    const onSaved = vi.fn()
    render(<Harness serverValue={3} onSaved={onSaved} />)

    fireEvent.click(dec())
    expect(count()).toBe('2')

    await flushDebounce()
    expect(count()).toBe('3')
    expect(screen.getByRole('alert')).toHaveTextContent('Failed to save worker count')
    expect(onSaved).not.toHaveBeenCalled()
  })

  it('adopts a new server value when no edit is pending', () => {
    const { rerender } = render(<Harness serverValue={2} />)
    rerender(<Harness serverValue={4} />)
    expect(count()).toBe('4')
  })

  it('ignores a polled server value while an edit is pending', () => {
    const { rerender } = render(<Harness serverValue={2} />)
    fireEvent.click(inc())
    rerender(<Harness serverValue={7} />)
    expect(count()).toBe('3')
  })
})
