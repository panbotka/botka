import { describe, it, expect } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useInputHistory } from './useInputHistory'

describe('useInputHistory', () => {
  it('starts with no history', () => {
    const { result } = renderHook(() => useInputHistory())
    expect(result.current.navigateUp('')).toBeNull()
    expect(result.current.navigateDown()).toBeNull()
  })

  it('stores pushed items', () => {
    const { result } = renderHook(() => useInputHistory())

    act(() => result.current.push('hello'))

    expect(result.current.navigateUp('')).toBe('hello')
  })

  it('navigates up through history', () => {
    const { result } = renderHook(() => useInputHistory())

    act(() => {
      result.current.push('first')
      result.current.push('second')
      result.current.push('third')
    })

    // Navigate up from newest to oldest
    expect(result.current.navigateUp('')).toBe('third')
    expect(result.current.navigateUp('')).toBe('second')
    expect(result.current.navigateUp('')).toBe('first')
    // At oldest — returns null
    expect(result.current.navigateUp('')).toBeNull()
  })

  it('navigates down back to newer items', () => {
    const { result } = renderHook(() => useInputHistory())

    act(() => {
      result.current.push('first')
      result.current.push('second')
    })

    // Navigate up to oldest
    result.current.navigateUp('')
    result.current.navigateUp('')

    // Navigate back down
    expect(result.current.navigateDown()).toBe('second')
  })

  it('restores draft when navigating past newest', () => {
    const { result } = renderHook(() => useInputHistory())

    act(() => result.current.push('saved'))

    // Start navigating with "draft text" as current input
    result.current.navigateUp('draft text')
    // Navigate down past newest — should restore draft
    expect(result.current.navigateDown()).toBe('draft text')
  })

  it('ignores empty pushes', () => {
    const { result } = renderHook(() => useInputHistory())

    act(() => {
      result.current.push('')
      result.current.push('   ')
    })

    expect(result.current.navigateUp('')).toBeNull()
  })

  it('skips consecutive duplicates', () => {
    const { result } = renderHook(() => useInputHistory())

    act(() => {
      result.current.push('same')
      result.current.push('same')
      result.current.push('same')
    })

    result.current.navigateUp('')
    // Only one entry, so next up returns null
    expect(result.current.navigateUp('')).toBeNull()
  })

  it('reset clears navigation state', () => {
    const { result } = renderHook(() => useInputHistory())

    act(() => {
      result.current.push('item')
    })

    result.current.navigateUp('')
    act(() => result.current.reset())

    // After reset, navigateDown does nothing
    expect(result.current.navigateDown()).toBeNull()
  })
})
