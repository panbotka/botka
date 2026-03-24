import type { Project, Task, Thread, RunnerStatus, UsageInfo } from '../types'

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
