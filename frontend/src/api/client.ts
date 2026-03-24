import type { Project, Task, Thread, ThreadDetail, RunnerStatus, UsageInfo, Persona, Tag, Memory, SearchResult } from '../types'

const BASE_URL = '/api/v1'

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  })
  if (!res.ok) {
    let message = res.statusText
    try {
      const body = await res.json()
      if (body.error) message = body.error
    } catch {
      // use statusText
    }
    throw new ApiError(res.status, message)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

async function requestData<T>(path: string, init?: RequestInit): Promise<T> {
  const body = await request<{ data: T }>(path, init)
  return body.data
}

// Projects

export function fetchProjects(): Promise<Project[]> {
  return requestData<Project[]>('/projects')
}

export function fetchProject(id: string): Promise<Project> {
  return requestData<Project>(`/projects/${id}`)
}

export function updateProject(id: string, data: Partial<Project>): Promise<Project> {
  return requestData<Project>(`/projects/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export function scanProjects(): Promise<{ discovered: number; new: number; deactivated: number }> {
  return requestData('/projects/scan', { method: 'POST' })
}

// Tasks

export function fetchTasks(params?: {
  status?: string
  project_id?: string
  limit?: number
  offset?: number
}): Promise<{ data: Task[]; total: number }> {
  const search = new URLSearchParams()
  if (params?.status) search.set('status', params.status)
  if (params?.project_id) search.set('project_id', params.project_id)
  if (params?.limit != null) search.set('limit', String(params.limit))
  if (params?.offset != null) search.set('offset', String(params.offset))
  const qs = search.toString()
  return request(`/tasks${qs ? `?${qs}` : ''}`)
}

export function createTask(data: {
  title: string
  spec: string
  project_id: string
  priority?: number
  status?: string
}): Promise<Task> {
  return requestData<Task>('/tasks', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export function fetchTask(id: string): Promise<Task> {
  return requestData<Task>(`/tasks/${id}`)
}

export function updateTask(id: string, data: Partial<Task>): Promise<Task> {
  return requestData<Task>(`/tasks/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export function deleteTask(id: string): Promise<void> {
  return request<void>(`/tasks/${id}`, { method: 'DELETE' })
}

export function reorderTasks(items: { id: string; priority: number }[]): Promise<void> {
  return requestData<void>('/tasks/reorder', {
    method: 'POST',
    body: JSON.stringify(items),
  })
}

export function retryTask(id: string): Promise<Task> {
  return requestData<Task>(`/tasks/${id}/retry`, { method: 'POST' })
}

export function batchUpdateTaskStatus(ids: string[], status: string): Promise<{ updated: number }> {
  return requestData<{ updated: number }>('/tasks/batch-status', {
    method: 'POST',
    body: JSON.stringify({ ids, status }),
  })
}

// Runner

export function fetchRunnerStatus(): Promise<RunnerStatus> {
  return requestData<RunnerStatus>('/runner/status')
}

export function startRunner(count?: number): Promise<void> {
  return requestData<void>('/runner/start', {
    method: 'POST',
    body: count ? JSON.stringify({ count }) : undefined,
  })
}

export function pauseRunner(): Promise<void> {
  return requestData<void>('/runner/pause', { method: 'POST' })
}

export function stopRunner(): Promise<void> {
  return requestData<void>('/runner/stop', { method: 'POST' })
}

export function refreshUsage(): Promise<UsageInfo> {
  return requestData<UsageInfo>('/runner/usage/refresh', { method: 'POST' })
}

// Threads

export function fetchThreads(): Promise<Thread[]> {
  return requestData<Thread[]>('/threads')
}

export function fetchThread(id: number): Promise<Thread> {
  return requestData<Thread>(`/threads/${id}`)
}

export function createThread(data: Partial<Thread>): Promise<Thread> {
  return requestData<Thread>('/threads', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export function deleteThread(id: number): Promise<void> {
  return request<void>(`/threads/${id}`, { method: 'DELETE' })
}

// Thread operations (extended)

export function updateThread(id: number, data: Partial<Thread>): Promise<Thread> {
  return requestData<Thread>(`/threads/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export function getThread(id: number): Promise<ThreadDetail> {
  return requestData<ThreadDetail>(`/threads/${id}`)
}

export function pinThread(id: number): Promise<void> {
  return request<void>(`/threads/${id}/pin`, { method: 'PUT' })
}

export function unpinThread(id: number): Promise<void> {
  return request<void>(`/threads/${id}/pin`, { method: 'DELETE' })
}

export function archiveThread(id: number): Promise<void> {
  return request<void>(`/threads/${id}/archive`, { method: 'PUT' })
}

export function unarchiveThread(id: number): Promise<void> {
  return request<void>(`/threads/${id}/archive`, { method: 'DELETE' })
}

export function clearMessages(id: number): Promise<void> {
  return request<void>(`/threads/${id}/messages`, { method: 'DELETE' })
}

export function clearSession(id: number): Promise<void> {
  return request<void>(`/threads/${id}/session`, { method: 'DELETE' })
}

export function updateModel(id: number, model: string): Promise<void> {
  return requestData<void>(`/threads/${id}/model`, {
    method: 'PUT',
    body: JSON.stringify({ model }),
  })
}

export function switchBranch(threadId: number, forkMessageId: number, childId: number): Promise<void> {
  return requestData<void>(`/threads/${threadId}/branch`, {
    method: 'PUT',
    body: JSON.stringify({ fork_message_id: forkMessageId, child_id: childId }),
  })
}

export function updateThreadTags(id: number, tagIds: number[]): Promise<void> {
  return requestData<void>(`/threads/${id}/tags`, {
    method: 'PUT',
    body: JSON.stringify({ tag_ids: tagIds }),
  })
}

export function updateThreadProject(id: number, projectId: string | null): Promise<void> {
  return requestData<void>(`/threads/${id}/project`, {
    method: 'PUT',
    body: JSON.stringify({ project_id: projectId }),
  })
}

// Search

export function searchMessages(query: string): Promise<SearchResult[]> {
  return requestData<SearchResult[]>(`/search?q=${encodeURIComponent(query)}`)
}

// Tags

export function fetchTags(): Promise<Tag[]> {
  return requestData<Tag[]>('/tags')
}

export function createTag(data: Partial<Tag>): Promise<Tag> {
  return requestData<Tag>('/tags', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export function updateTag(id: number, data: Partial<Tag>): Promise<Tag> {
  return requestData<Tag>(`/tags/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export function deleteTag(id: number): Promise<void> {
  return request<void>(`/tags/${id}`, { method: 'DELETE' })
}

// Personas

export function fetchPersonas(): Promise<Persona[]> {
  return requestData<Persona[]>('/personas')
}

export function createPersona(data: Partial<Persona>): Promise<Persona> {
  return requestData<Persona>('/personas', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export function updatePersona(id: number, data: Partial<Persona>): Promise<Persona> {
  return requestData<Persona>(`/personas/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export function deletePersona(id: number): Promise<void> {
  return request<void>(`/personas/${id}`, { method: 'DELETE' })
}

// Memories

export function fetchMemories(): Promise<Memory[]> {
  return requestData<Memory[]>('/memories')
}

export function createMemory(content: string): Promise<Memory> {
  return requestData<Memory>('/memories', {
    method: 'POST',
    body: JSON.stringify({ content }),
  })
}

export function updateMemory(id: string, content: string): Promise<Memory> {
  return requestData<Memory>(`/memories/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ content }),
  })
}

export function deleteMemory(id: string): Promise<void> {
  return request<void>(`/memories/${id}`, { method: 'DELETE' })
}

// Processes

export function listProcesses(): Promise<{ thread_id: number; thread_title: string; started_at: string; duration_sec: number }[]> {
  return requestData('/processes')
}

export function killProcess(threadId: number): Promise<void> {
  return request<void>(`/processes/${threadId}`, { method: 'DELETE' })
}

// Status & Models

export function getStatus(): Promise<{ default_model: string }> {
  return requestData('/status')
}

export function getModels(): Promise<{ models: string[] }> {
  return requestData('/models')
}

export function getTranscribeStatus(): Promise<{ enabled: boolean }> {
  return requestData('/transcribe/status')
}

export function transcribe(blob: Blob, lang?: string): Promise<string> {
  const formData = new FormData()
  formData.append('audio', blob)
  if (lang) formData.append('language', lang)
  return fetch(`${BASE_URL}/transcribe`, {
    method: 'POST',
    body: formData,
  }).then(async (res) => {
    if (!res.ok) throw new ApiError(res.status, 'Transcription failed')
    const data = await res.json()
    return data.data?.text || data.text || ''
  })
}

// Streaming

export interface StreamChunk {
  done?: boolean
  error?: string
  connectionLost?: boolean
  content?: string
  thinking?: string
  thinking_done?: { duration_ms: number }
  title?: string
  usage?: { prompt_tokens: number; completion_tokens: number; cost_usd?: number; input_tokens?: number; output_tokens?: number }
  tool_use?: { id: string; name: string; input: Record<string, unknown> }
  tool_result?: { tool_use_id: string; content: string; is_error: boolean }
  retry?: { attempt: number; max_attempts: number }
  memory_suggestion?: string
}

async function* parseSSE(response: Response): AsyncGenerator<StreamChunk> {
  const reader = response.body!.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const data = line.slice(6)
          if (data === '[DONE]') {
            yield { done: true }
            return
          }
          try {
            yield JSON.parse(data) as StreamChunk
          } catch {
            // skip malformed
          }
        }
      }
    }
  } finally {
    reader.releaseLock()
  }
}

export async function* streamChat(
  threadId: number,
  content: string,
  signal: AbortSignal,
  files?: File[],
  planMode?: boolean,
): AsyncGenerator<StreamChunk> {
  let body: BodyInit
  let headers: Record<string, string> = {}

  if (files && files.length > 0) {
    const formData = new FormData()
    formData.append('content', content)
    if (planMode) formData.append('plan_mode', 'true')
    for (const file of files) {
      formData.append('files', file)
    }
    body = formData
  } else {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify({ content, plan_mode: planMode || undefined })
  }

  const res = await fetch(`${BASE_URL}/threads/${threadId}/messages`, {
    method: 'POST',
    headers,
    body,
    signal,
  })

  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText)
    throw new Error(text || res.statusText)
  }

  yield* parseSSE(res)
}

export async function* streamRegenerate(
  threadId: number,
  signal: AbortSignal,
): AsyncGenerator<StreamChunk> {
  const res = await fetch(`${BASE_URL}/threads/${threadId}/regenerate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    signal,
  })

  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText)
    throw new Error(text || res.statusText)
  }

  yield* parseSSE(res)
}

export async function* streamEdit(
  threadId: number,
  messageId: number,
  content: string,
  signal: AbortSignal,
): AsyncGenerator<StreamChunk> {
  const res = await fetch(`${BASE_URL}/threads/${threadId}/messages/${messageId}/edit`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
    signal,
  })

  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText)
    throw new Error(text || res.statusText)
  }

  yield* parseSSE(res)
}

export async function* streamBranch(
  threadId: number,
  branchParentId: number,
  content: string,
  signal: AbortSignal,
): AsyncGenerator<StreamChunk> {
  const res = await fetch(`${BASE_URL}/threads/${threadId}/branch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ parent_id: branchParentId, content }),
    signal,
  })

  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText)
    throw new Error(text || res.statusText)
  }

  yield* parseSSE(res)
}

// Convenience object for use in hooks that call api.methodName()
export const api = {
  // Threads
  getThread,
  createThread,
  updateThread,
  deleteThread,
  fetchThreads,
  pinThread,
  unpinThread,
  archiveThread,
  unarchiveThread,
  clearMessages,
  clearSession,
  updateModel,
  switchBranch,
  updateThreadTags,
  updateThreadProject,
  // Search
  searchMessages,
  // Tags
  fetchTags,
  createTag,
  updateTag,
  deleteTag,
  // Personas
  fetchPersonas,
  createPersona,
  updatePersona,
  deletePersona,
  // Memories
  fetchMemories,
  createMemory,
  updateMemory,
  deleteMemory,
  // Processes
  listProcesses,
  killProcess,
  // Status
  getStatus,
  getModels,
  getTranscribeStatus,
  transcribe,
}
