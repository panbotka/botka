import { useState, useEffect, useCallback } from 'react'
import type { EffectiveSkill } from '../types'
import { fetchThreadSkills, setThreadSkills } from '../api/client'

interface UseThreadSkillsResult {
  skills: EffectiveSkill[]
  loading: boolean
  error: string | null
  toggle: (name: string) => Promise<void>
}

/**
 * Loads the skills a thread may use and toggles them.
 *
 * The server persists only the skills whose state deviates from the registry
 * default, so it always answers with the full effective list — we replace
 * local state with its response rather than trusting our optimistic guess.
 */
export function useThreadSkills(threadId: number): UseThreadSkillsResult {
  const [skills, setSkills] = useState<EffectiveSkill[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      setError(null)
      setSkills(await fetchThreadSkills(threadId))
    } catch {
      setError('Failed to load skills')
    } finally {
      setLoading(false)
    }
  }, [threadId])

  useEffect(() => {
    load()
  }, [load])

  const toggle = useCallback(
    async (name: string) => {
      const target = skills.find((s) => s.name === name)
      if (!target) return

      const newEnabled = !target.enabled
      setSkills((prev) => prev.map((s) => (s.name === name ? { ...s, enabled: newEnabled } : s)))

      const enabledNames = skills
        .filter((s) => (s.name === name ? newEnabled : s.enabled))
        .map((s) => s.name)

      try {
        setSkills(await setThreadSkills(threadId, enabledNames))
      } catch {
        setSkills((prev) => prev.map((s) => (s.name === name ? { ...s, enabled: !newEnabled } : s)))
        setError('Failed to update skills')
      }
    },
    [skills, threadId],
  )

  return { skills, loading, error, toggle }
}
