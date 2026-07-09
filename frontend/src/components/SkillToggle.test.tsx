import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import SkillToggle from './SkillToggle'
import type { EffectiveSkill } from '../types'

const fetchThreadSkills = vi.fn()
const setThreadSkills = vi.fn()

vi.mock('../api/client', () => ({
  fetchThreadSkills: (...args: unknown[]) => fetchThreadSkills(...args),
  setThreadSkills: (...args: unknown[]) => setThreadSkills(...args),
}))

function skill(overrides: Partial<EffectiveSkill> = {}): EffectiveSkill {
  return {
    name: 'golang-developer',
    description: 'Go conventions',
    source: 'user',
    default_enabled: true,
    enabled: true,
    overridden: false,
    ...overrides,
  }
}

function renderToggle() {
  return render(
    <MemoryRouter>
      <SkillToggle threadId={1} />
    </MemoryRouter>,
  )
}

describe('SkillToggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('lists skills with their source', async () => {
    fetchThreadSkills.mockResolvedValue([
      skill(),
      skill({ name: 'brainstorming', source: 'plugin:superpowers', enabled: false }),
    ])

    renderToggle()

    expect(await screen.findByText('golang-developer')).toBeTruthy()
    expect(screen.getByText('brainstorming')).toBeTruthy()
    // The "plugin:" prefix is stripped down to the owning plugin's name.
    expect(screen.getByText('superpowers')).toBeTruthy()
  })

  it('sends the full enabled set when a skill is turned off', async () => {
    fetchThreadSkills.mockResolvedValue([skill(), skill({ name: 'react-vite' })])
    setThreadSkills.mockResolvedValue([skill({ enabled: false, overridden: true }), skill({ name: 'react-vite' })])

    renderToggle()
    fireEvent.click(await screen.findByLabelText('Toggle golang-developer'))

    await waitFor(() => expect(setThreadSkills).toHaveBeenCalledWith(1, ['react-vite']))
  })

  it('sends the full enabled set when a skill is turned on', async () => {
    fetchThreadSkills.mockResolvedValue([skill({ enabled: false, overridden: true })])
    setThreadSkills.mockResolvedValue([skill()])

    renderToggle()
    fireEvent.click(await screen.findByLabelText('Toggle golang-developer'))

    await waitFor(() => expect(setThreadSkills).toHaveBeenCalledWith(1, ['golang-developer']))
  })

  it('reverts the switch when the update fails', async () => {
    fetchThreadSkills.mockResolvedValue([skill()])
    setThreadSkills.mockRejectedValue(new Error('boom'))

    renderToggle()
    fireEvent.click(await screen.findByLabelText('Toggle golang-developer'))

    expect(await screen.findByText('Failed to update skills')).toBeTruthy()
  })

  it('offers a settings link when no skills exist', async () => {
    fetchThreadSkills.mockResolvedValue([])

    renderToggle()

    expect(await screen.findByText('No skills found')).toBeTruthy()
    expect(screen.getByText('Manage in Settings')).toBeTruthy()
  })

  it('surfaces load failures', async () => {
    fetchThreadSkills.mockRejectedValue(new Error('nope'))

    renderToggle()

    expect(await screen.findByText('Failed to load skills')).toBeTruthy()
  })
})
