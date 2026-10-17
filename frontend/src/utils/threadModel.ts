import { api } from '../api/client'

/**
 * saveThreadModel switches a thread's model and, on success, tells the caller to
 * reload threads so every model label picks up the change. Shared by the model
 * picker in ThreadSettingsPanel and the mobile header ModelChip. Returns whether
 * the save succeeded; errors are swallowed like the rest of the thread settings.
 */
export async function saveThreadModel(
  threadId: number,
  model: string,
  onSaved: () => void,
): Promise<boolean> {
  try {
    await api.updateModel(threadId, model)
    onSaved()
    return true
  } catch {
    return false
  }
}
