import { useEffect, useState } from 'react'
import { api } from '../api/client'

/**
 * useAvailableModels loads the configured model list (`AVAILABLE_MODELS`) once.
 * `models` is null while loading; `failed` is true when the request errored, in
 * which case callers fall back to free-form input or a message.
 */
export function useAvailableModels(): { models: string[] | null; failed: boolean } {
  const [models, setModels] = useState<string[] | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let active = true
    api.getModels()
      .then((res) => { if (active) setModels(res.models) })
      .catch(() => { if (active) setFailed(true) })
    return () => { active = false }
  }, [])

  return { models, failed }
}
