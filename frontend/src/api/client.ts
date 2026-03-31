import type { Project, Task, Thread, ThreadDetail, ThreadSource, RunnerStatus, UsageInfo, Persona, Tag, Memory, SearchResult, GitCommit, GitStatus, ProjectStats, RunningCommandStatus, TaskStats, GlobalSearchResults, CostAnalytics, ServerSettings, Message, BoxStatus } from '../types'

const BASE_URL = '/api/v1'

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

// Callback invoked on 401 responses to trigger auth redirect.
let onUnauthorized: (() => void) | null = null

export function setOnUnauthorized(cb: () => void) {
  onUnauthorized = cb
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
    // Auto-redirect on 401 (except for /auth/me which is used to check auth state).
    if (res.status === 401 && !path.startsWith('/auth/me') && onUnauthorized) {
      onUnauthorized()
    }
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

export function fetchProjectGitLog(id: string): Promise<GitCommit[]> {
  return requestData<GitCommit[]>(`/projects/${id}/git-log`)
}

export function fetchProjectGitStatus(id: string): Promise<GitStatus> {
  return requestData<GitStatus>(`/projects/${id}/git-status`)
}

export function fetchProjectStats(id: string): Promise<ProjectStats> {
  return requestData<ProjectStats>(`/projects/${id}/stats`)
}

// Project Commands

export function runProjectCommand(id: string, command: 'dev' | 'deploy'): Promise<{ pid: number; command_type: string }> {
  return requestData(`/projects/${id}/run-command`, {
    method: 'POST',
    body: JSON.stringify({ command }),
  })
}

export function fetchProjectCommands(id: string): Promise<RunningCommandStatus[]> {
  return requestData<RunningCommandStatus[]>(`/projects/${id}/commands`)
}

export function killProjectCommand(projectId: string, pid: number): Promise<void> {
  return request<void>(`/projects/${projectId}/commands/${pid}`, { method: 'DELETE' })
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

export function killTask(id: string): Promise<{ message: string }> {
  return requestData<{ message: string }>(`/tasks/${id}/kill`, { method: 'POST' })
}

export function batchUpdateTaskStatus(ids: string[], status: string): Promise<{ updated: number }> {
  return requestData<{ updated: number }>('/tasks/batch-status', {
    method: 'POST',
    body: JSON.stringify({ ids, status }),
  })
}

export function fetchTaskStats(): Promise<TaskStats> {
  return requestData<TaskStats>('/tasks/stats')
}

export function fetchTaskRawOutput(id: string): Promise<{ execution_id: string; attempt: number; raw_output: string }> {
  return requestData<{ execution_id: string; attempt: number; raw_output: string }>(`/tasks/${id}/output/raw`)
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

export function fetchThreads(includeArchived?: boolean): Promise<Thread[]> {
  const qs = includeArchived ? '?include_archived=true' : ''
  return requestData<Thread[]>(`/threads${qs}`)
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

export function toggleMessageHidden(messageId: number): Promise<Message> {
  return requestData<Message>(`/messages/${messageId}/hide`, { method: 'PATCH' })
}

export function clearSession(id: number): Promise<void> {
  return request<void>(`/threads/${id}/session/clear`, { method: 'POST' })
}

export function interruptThread(id: number): Promise<void> {
  return request<void>(`/threads/${id}/interrupt`, { method: 'POST' })
}

export function newSession(id: number): Promise<void> {
  return request<void>(`/threads/${id}/session/new`, { method: 'POST' })
}

export interface SessionHealthData {
  active: boolean
  total_input_tokens?: number
  total_output_tokens?: number
  estimated_context_tokens?: number
  context_limit?: number
  context_usage_pct?: number
  model?: string
  started_at?: string
  message_count?: number
}

export function fetchSessionHealth(id: number): Promise<SessionHealthData> {
  return requestData<SessionHealthData>(`/threads/${id}/session-health`)
}

export function renameThread(id: number, title: string): Promise<Thread> {
  return updateThread(id, { title } as Partial<Thread>)
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

export function updateCustomContext(threadId: number, customContext: string): Promise<void> {
  return request<void>(`/threads/${threadId}/custom-context`, {
    method: 'PUT',
    body: JSON.stringify({ custom_context: customContext }),
  })
}

// Thread Sources

export function fetchThreadSources(threadId: number): Promise<ThreadSource[]> {
  return requestData<ThreadSource[]>(`/threads/${threadId}/sources`)
}

export function createThreadSource(threadId: number, data: { url: string; label?: string }): Promise<ThreadSource> {
  return requestData<ThreadSource>(`/threads/${threadId}/sources`, {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export function updateThreadSource(threadId: number, sourceId: number, data: { url: string; label?: string }): Promise<ThreadSource> {
  return requestData<ThreadSource>(`/threads/${threadId}/sources/${sourceId}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export function deleteThreadSource(threadId: number, sourceId: number): Promise<void> {
  return request<void>(`/threads/${threadId}/sources/${sourceId}`, { method: 'DELETE' })
}

export function reorderThreadSources(threadId: number, ids: number[]): Promise<void> {
  return requestData<void>(`/threads/${threadId}/sources/reorder`, {
    method: 'PUT',
    body: JSON.stringify({ ids }),
  })
}

// Search

export function searchMessages(query: string): Promise<SearchResult[]> {
  return requestData<SearchResult[]>(`/search?q=${encodeURIComponent(query)}`)
}

export function globalSearch(query: string, limit?: number): Promise<GlobalSearchResults> {
  const params = new URLSearchParams({ q: query })
  if (limit != null) params.set('limit', String(limit))
  return requestData<GlobalSearchResults>(`/search/global?${params}`)
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

export function getTagThreadCount(id: number): Promise<number> {
  return requestData<{ count: number }>(`/tags/${id}/threads/count`).then((r) => r.count)
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

// Server settings

export function fetchServerSettings(): Promise<ServerSettings> {
  return requestData<ServerSettings>('/settings')
}

export function updateServerSettings(settings: Partial<ServerSettings>): Promise<ServerSettings> {
  return requestData<ServerSettings>('/settings', {
    method: 'PUT',
    body: JSON.stringify(settings),
  })
}

// Maintenance

export function purgeTaskOutputs(): Promise<{ purged: number }> {
  return requestData<{ purged: number }>('/settings/task-outputs', { method: 'DELETE' })
}

// Analytics

export function fetchCostAnalytics(days?: number): Promise<CostAnalytics> {
  const params = new URLSearchParams()
  if (days != null) params.set('days', String(days))
  const qs = params.toString()
  return requestData<CostAnalytics>(`/analytics/cost${qs ? `?${qs}` : ''}`)
}

// Streaming

export interface StreamChunk {
  done?: boolean
  error?: string
  error_raw?: string
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
  attachments?: { id: number; message_id: number; stored_name: string; original_name: string; mime_type: string; size: number; url: string; created_at: string }[]
}

async function* parseSSE(response: Response): AsyncGenerator<StreamChunk> {
  const reader = response.body!.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let currentEventType = ''

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (line.startsWith('event: ')) {
          currentEventType = line.slice(7).trim()
          continue
        }
        if (line.startsWith('data: ')) {
          const data = line.slice(6)
          if (data === '[DONE]') {
            yield { done: true }
            return
          }
          try {
            const parsed = JSON.parse(data)
            switch (currentEventType) {
              case 'tool_use':
                yield { tool_use: parsed } as StreamChunk
                break
              case 'tool_result':
                yield { tool_result: parsed } as StreamChunk
                break
              case 'thinking':
                yield { thinking: parsed.content } as StreamChunk
                break
              case 'usage':
                yield { usage: parsed } as StreamChunk
                break
              case 'error':
                yield { error: parsed.error, error_raw: parsed.raw } as StreamChunk
                break
              case 'title':
                yield { title: parsed.title } as StreamChunk
                break
              case 'attachments':
                yield { attachments: parsed } as StreamChunk
                break
              default:
                yield parsed as StreamChunk
                break
            }
          } catch {
            // skip malformed
          }
          currentEventType = ''
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

export async function* streamSubscribe(
  threadId: number,
  signal: AbortSignal,
): AsyncGenerator<StreamChunk> {
  const res = await fetch(`${BASE_URL}/threads/${threadId}/stream/subscribe`, {
    signal,
  })

  if (!res.ok) return

  yield* parseSSE(res)
}

// Auth

export type UserRole = 'admin' | 'external'

export interface AuthUser {
  id: number
  username: string
  role: UserRole
  passkey_count: number
}

export interface ExternalUser {
  id: number
  username: string
  role: UserRole
  thread_count: number
  created_at: string
}

export interface UserThreadAccess {
  id: number
  thread_id: number
  thread_title: string
  created_at: string
}

export function authMe(): Promise<AuthUser> {
  return requestData<AuthUser>('/auth/me')
}

export function authLogin(username: string, password: string): Promise<AuthUser> {
  return requestData<AuthUser>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export function authLogout(): Promise<void> {
  return request<void>('/auth/logout', { method: 'POST' })
}

export function authChangePassword(currentPassword: string, newPassword: string): Promise<void> {
  return requestData<void>('/auth/password', {
    method: 'PUT',
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  })
}

// Passkeys

export interface PasskeyInfo {
  id: number
  name: string
  created_at: string
}

export function fetchPasskeys(): Promise<PasskeyInfo[]> {
  return requestData<PasskeyInfo[]>('/auth/passkeys')
}

export function deletePasskey(id: number): Promise<void> {
  return request<void>(`/auth/passkeys/${id}`, { method: 'DELETE' })
}

export async function passkeyRegisterBegin(): Promise<{ data: PublicKeyCredentialCreationOptions }> {
  const res = await request<{ data: { publicKey: PublicKeyCredentialCreationOptions } }>('/auth/passkey/register/begin', {
    method: 'POST',
  })
  return { data: res.data.publicKey }
}

export function passkeyRegisterFinish(credential: Credential, name: string): Promise<PasskeyInfo> {
  const pkCred = credential as PublicKeyCredential
  const response = pkCred.response as AuthenticatorAttestationResponse
  return requestData<PasskeyInfo>(`/auth/passkey/register/finish?name=${encodeURIComponent(name)}`, {
    method: 'POST',
    body: JSON.stringify({
      id: pkCred.id,
      rawId: bufferToBase64url(pkCred.rawId),
      type: pkCred.type,
      response: {
        attestationObject: bufferToBase64url(response.attestationObject),
        clientDataJSON: bufferToBase64url(response.clientDataJSON),
      },
    }),
  })
}

export async function passkeyLoginBegin(): Promise<{ data: PublicKeyCredentialRequestOptions }> {
  const res = await request<{ data: { publicKey: PublicKeyCredentialRequestOptions } }>('/auth/passkey/login/begin', {
    method: 'POST',
  })
  return { data: res.data.publicKey }
}

export function passkeyLoginFinish(credential: Credential): Promise<AuthUser> {
  const pkCred = credential as PublicKeyCredential
  const response = pkCred.response as AuthenticatorAssertionResponse
  return requestData<AuthUser>('/auth/passkey/login/finish', {
    method: 'POST',
    body: JSON.stringify({
      id: pkCred.id,
      rawId: bufferToBase64url(pkCred.rawId),
      type: pkCred.type,
      response: {
        authenticatorData: bufferToBase64url(response.authenticatorData),
        clientDataJSON: bufferToBase64url(response.clientDataJSON),
        signature: bufferToBase64url(response.signature),
        userHandle: response.userHandle ? bufferToBase64url(response.userHandle) : undefined,
      },
    }),
  })
}

function bufferToBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  bytes.forEach((b) => { binary += String.fromCharCode(b) })
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

// User management (admin only)

export function fetchUsers(): Promise<ExternalUser[]> {
  return requestData<ExternalUser[]>('/users')
}

export function createUser(username: string, password: string): Promise<ExternalUser> {
  return requestData<ExternalUser>('/users', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export function deleteUser(id: number): Promise<void> {
  return request<void>(`/users/${id}`, { method: 'DELETE' })
}

export function resetUserPassword(id: number, password: string): Promise<void> {
  return requestData<void>(`/users/${id}/password`, {
    method: 'PUT',
    body: JSON.stringify({ password }),
  })
}

export function fetchUserThreads(userId: number): Promise<UserThreadAccess[]> {
  return requestData<UserThreadAccess[]>(`/users/${userId}/threads`)
}

export function grantUserThread(userId: number, threadId: number): Promise<void> {
  return requestData<void>(`/users/${userId}/threads`, {
    method: 'POST',
    body: JSON.stringify({ thread_id: threadId }),
  })
}

export function revokeUserThread(userId: number, threadId: number): Promise<void> {
  return request<void>(`/users/${userId}/threads/${threadId}`, { method: 'DELETE' })
}

// Box server

export function fetchBoxStatus(): Promise<BoxStatus> {
  return requestData<BoxStatus>('/box/status')
}

export function wakeBox(): Promise<{ message: string }> {
  return requestData<{ message: string }>('/box/wake', { method: 'POST' })
}

export function shutdownBox(): Promise<{ message: string }> {
  return requestData<{ message: string }>('/box/shutdown', { method: 'POST' })
}

export function startBoxService(name: string): Promise<{ message: string }> {
  return requestData<{ message: string }>(`/box/services/${encodeURIComponent(name)}/start`, { method: 'POST' })
}

export function stopBoxService(name: string): Promise<{ message: string }> {
  return requestData<{ message: string }>(`/box/services/${encodeURIComponent(name)}/stop`, { method: 'POST' })
}

// Convenience object for use in hooks that call api.methodName()
export const api = {
  // Projects
  fetchProjects,
  fetchProject,
  updateProject,
  scanProjects,
  fetchProjectGitLog,
  fetchProjectGitStatus,
  fetchProjectStats,
  runProjectCommand,
  fetchProjectCommands,
  killProjectCommand,
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
  toggleMessageHidden,
  clearSession,
  interruptThread,
  newSession,
  fetchSessionHealth,
  renameThread,
  updateModel,
  switchBranch,
  updateThreadTags,
  updateThreadProject,
  // Thread Sources
  fetchThreadSources,
  createThreadSource,
  updateThreadSource,
  deleteThreadSource,
  reorderThreadSources,
  // Search
  searchMessages,
  globalSearch,
  // Tags
  fetchTags,
  createTag,
  updateTag,
  deleteTag,
  getTagThreadCount,
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
  // Task Stats
  fetchTaskStats,
  // Server settings
  fetchServerSettings,
  updateServerSettings,
  // Maintenance
  purgeTaskOutputs,
  // Analytics
  fetchCostAnalytics,
  // Status
  getStatus,
  getModels,
  getTranscribeStatus,
  transcribe,
  // User management
  fetchUsers,
  createUser,
  deleteUser,
  resetUserPassword,
  fetchUserThreads,
  grantUserThread,
  revokeUserThread,
  // Box
  fetchBoxStatus,
  wakeBox,
  shutdownBox,
  startBoxService,
  stopBoxService,
}
