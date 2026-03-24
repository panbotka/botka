import type { Project, Task, Thread, RunnerStatus } from '../types'

const BASE = '/api/v1'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) throw new Error(await res.text())
  const json = await res.json()
  return json.data ?? json
}

function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, { method: 'POST', body: JSON.stringify(body) })
}

function put<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, { method: 'PUT', body: JSON.stringify(body) })
}

function del<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'DELETE' })
}

export const api = {
  // Projects
  getProjects: () => request<Project[]>('/projects'),
  getProject: (id: string) => request<Project>(`/projects/${id}`),
  createProject: (data: Partial<Project>) => post<Project>('/projects', data),
  updateProject: (id: string, data: Partial<Project>) => put<Project>(`/projects/${id}`, data),
  deleteProject: (id: string) => del<void>(`/projects/${id}`),

  // Tasks
  getTasks: (params?: Record<string, string>) =>
    request<Task[]>('/tasks' + (params ? '?' + new URLSearchParams(params) : '')),
  getTask: (id: string) => request<Task>(`/tasks/${id}`),
  createTask: (data: Partial<Task>) => post<Task>('/tasks', data),
  updateTask: (id: string, data: Partial<Task>) => put<Task>(`/tasks/${id}`, data),
  deleteTask: (id: string) => del<void>(`/tasks/${id}`),

  // Runner
  getRunnerStatus: () => request<RunnerStatus>('/runner/status'),

  // Threads
  getThreads: () => request<Thread[]>('/threads'),
  getThread: (id: number) => request<Thread>(`/threads/${id}`),
  createThread: (data: Partial<Thread>) => post<Thread>('/threads', data),
  deleteThread: (id: number) => del<void>(`/threads/${id}`),
}
