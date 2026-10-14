import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import ProjectFilter, { filterProjectNames } from './ProjectFilter'

const names = ['botka', 'fio-banka', 'kukatko', 'Kukátko-old', 'sekretarka']

describe('filterProjectNames', () => {
  it('keeps everything for an empty query', () => {
    expect(filterProjectNames(names, '  ')).toEqual(names)
  })
  it('ranks prefix before substring before fuzzy', () => {
    expect(filterProjectNames(['xbank', 'banka', 'bxaxnxk'], 'bank')).toEqual(['banka', 'xbank', 'bxaxnxk'])
  })
  it('ignores case and diacritics', () => {
    expect(filterProjectNames(names, 'KUKAT')).toEqual(['kukatko', 'Kukátko-old'])
  })
  it('drops non-matches', () => {
    expect(filterProjectNames(names, 'zzz')).toEqual([])
  })
})

describe('ProjectFilter', () => {
  it('filters as you type and picks the best match with Enter', () => {
    const onChange = vi.fn()
    render(<ProjectFilter names={names} value={null} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: /All projects/ }))
    const input = screen.getByRole('combobox')
    fireEvent.change(input, { target: { value: 'sekr' } })
    expect(screen.getAllByRole('option').map((o) => o.textContent)).toEqual(['sekretarka'])
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onChange).toHaveBeenCalledWith('sekretarka')
  })

  it('offers "All projects" to clear the filter', () => {
    const onChange = vi.fn()
    render(<ProjectFilter names={names} value="botka" onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: /botka/ }))
    fireEvent.click(screen.getByRole('option', { name: /All projects/ }))
    expect(onChange).toHaveBeenCalledWith(null)
  })

  it('moves the highlight with arrow keys', () => {
    const onChange = vi.fn()
    render(<ProjectFilter names={names} value={null} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: /All projects/ }))
    const input = screen.getByRole('combobox')
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onChange).toHaveBeenCalledWith('fio-banka')
  })
})
