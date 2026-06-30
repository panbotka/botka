// ── Projects (unified from Saiduler projects + Chatovadlo folders) ──

export type BranchStrategy = 'main' | 'feature_branch'

export interface Project {
  id: string
  name: string
  path: string
  branch_strategy: BranchStrategy
  verification_command?: string
  dev_command?: string
  deploy_command?: string
  dev_port?: number
  deploy_port?: number
  active: boolean
  claude_md: string
  sort_order: number
  created_at: string
  updated_at: string
  task_counts?: Record<string, number>
}

// ── Project detail types ──

export interface GitCommit {
  hash: string
  author: string
  date: string
  message: string
}

export interface ChangedFile {
  path: string
  status: string
}

export interface GitStatus {
  branch: string
  clean: boolean
  changed_files: ChangedFile[]
  diff_stat: string
  ahead: number
  ahead_remote: string
}

export interface RunningCommandStatus {
  pid: number
  port?: number
  command_type: string
  command: string
  started_at: string
  alive: boolean
}

export interface ProjectStats {
  total: number
  by_status: Record<string, number>
  avg_duration_ms: number | null
  success_rate: number | null
  total_cost_usd: number | null
}

export interface TaskStatsTopProject {
  id: string
  name: string
  count: number
}

export interface TaskStats {
  total: number
  by_status: Record<string, number>
  completed_today: number
  completed_week: number
  success_rate: number | null
  avg_duration_ms: number | null
  total_cost_usd: number | null
  top_project: TaskStatsTopProject | null
}

// ── Tasks (from Saiduler) ──

export type TaskStatus = 'pending' | 'queued' | 'running' | 'done' | 'failed' | 'needs_review' | 'cancelled' | 'deleted'

export interface Task {
  id: string
  title: string
  spec: string
  status: TaskStatus
  priority: number
  project_id: string
  project_name?: string
  project?: Project
  failure_reason: string | null
  failure_summary?: string | null
  retry_count: number
  executions?: TaskExecution[]
  started_at: string | null
  completed_at: string | null
  input_tokens: number | null
  output_tokens: number | null
  cache_read_tokens: number | null
  cache_creation_tokens: number | null
  cost_usd: number | null
  model?: string | null
  base_commit_sha?: string | null
  head_commit_sha?: string | null
  schedule_id?: number | null
  schedule?: TaskSchedule | null
  tags?: TaskTag[]
  notes_count?: number
  created_at: string
  updated_at: string
}

export interface TaskTag {
  id: number
  name: string
  color: string
  created_at: string
}

export interface TaskNote {
  id: number
  task_id: string
  body: string
  author: string
  created_at: string
  updated_at: string
}

export interface TaskDiffStats {
  files_changed: number
  insertions: number
  deletions: number
}

export interface TaskDiff {
  diff: string
  stats: TaskDiffStats
  truncated: boolean
}

export interface ProjectUsage {
  task_count: number
  total_input_tokens: number | null
  total_output_tokens: number | null
  total_cache_read_tokens: number | null
  total_cache_creation_tokens: number | null
  total_cost_usd: number | null
  avg_cost_per_task_usd: number | null
}

export interface ProjectMetricsDay {
  date: string  // UTC YYYY-MM-DD; convert to local time client-side
  count: number
}

export interface ProjectMetricsFailure {
  reason: string
  count: number
}

export interface ProjectMetricsDuration {
  task_id: string
  duration_ms: number
  completed_at: string  // UTC ISO timestamp
}

export interface ProjectMetrics {
  enough_data: boolean
  total: number
  by_status: Record<string, number>
  success_rate_30d: number | null
  avg_duration_ms_30d: number | null
  tasks_per_day: ProjectMetricsDay[]
  top_failures: ProjectMetricsFailure[]
  last_durations: ProjectMetricsDuration[]
  generated_at: string  // UTC ISO timestamp
}

export interface TaskExecution {
  id: string
  task_id: string
  attempt: number
  started_at: string
  finished_at: string | null
  exit_code: number | null
  cost_usd: number | null
  duration_ms: number | null
  summary: string | null
  error_message: string | null
}

export type RunnerStateValue = 'running' | 'paused' | 'stopped'

export interface RunnerStatus {
  state: RunnerStateValue
  active_tasks: ActiveTaskInfo[]
  max_workers: number
  draining: boolean
  usage: UsageInfo | null
  task_limit: number
  completed_count: number
  paused_until: string | null
  pause_reason: string | null
  pause_source: string | null
}

export interface ServerSettings {
  max_workers: number
}

export interface UsageInfo {
  five_hour_pct: number
  seven_day_pct: number
  resets_at: string
  last_checked: string
  age_seconds: number
  stale: boolean
  threshold_5h: number
  threshold_7d: number
}

export interface ActiveTaskInfo {
  task_id: string
  task_title: string
  project_name: string
  started_at: string
  orphaned?: boolean
}

// ── Threads (from Chatovadlo) ──

export interface Thread {
  id: number
  title: string
  model: string
  system_prompt: string
  custom_context: string
  persona_id?: number
  persona_icon?: string
  persona_name?: string
  project_id?: string
  folder_id?: number | null
  pinned: boolean
  archived: boolean
  color?: string
  claude_session_id?: string
  tags?: Tag[]
  created_at: string
  updated_at: string
  total_cost_usd?: number | null
  last_message_preview?: string
  last_message_at?: string
  signal_bridge_active?: boolean
  parent_thread_id?: number | null
  forked_from_message_id?: number | null
}

export interface ThreadFolder {
  id: number
  name: string
  parent_id: number | null
  position: number
  thread_count: number
  children: ThreadFolder[]
  created_at: string
  updated_at: string
}

export interface SignalBridge {
  id: number
  thread_id: number
  group_id: string
  group_name: string
  active: boolean
  created_at: string
  updated_at: string
}

export interface SignalGroup {
  id: string
  name: string
  member_count: number
}

export interface ThreadSource {
  id: number
  thread_id: number
  url: string
  label: string
  position: number
  created_at: string
  updated_at: string
}

export interface PersistedToolCall {
  name: string
  input: Record<string, unknown>
}

export interface Message {
  id: number
  thread_id: number
  role: 'user' | 'assistant' | 'system'
  content: string
  parent_id?: number
  thinking?: string
  thinking_duration_ms?: number
  prompt_tokens?: number
  completion_tokens?: number
  cost_usd?: number | null
  tool_calls?: PersistedToolCall[]
  hidden?: boolean
  attachments?: Attachment[]
  created_at: string
}

export interface Attachment {
  id: number
  message_id: number
  stored_name: string
  original_name: string
  mime_type: string
  size: number
  url: string
  created_at: string
}

export interface ThreadDetail {
  thread: Thread
  messages: Message[]
  fork_points?: Record<string, ForkPoint>
}

export interface ForkChild {
  id: number
  preview: string
  role: string
  created_at: string
}

export interface ForkPoint {
  children: ForkChild[]
  active_index: number
}

// ── Supporting (from Chatovadlo) ──

export interface Persona {
  id: number
  name: string
  system_prompt: string
  default_model: string
  icon: string
  starter_message: string
  sort_order: number
  created_at: string
  updated_at: string
}

export interface Tag {
  id: number
  name: string
  color: string
  created_at: string
}

export interface Memory {
  id: string
  content: string
  created_at: string
  updated_at: string
}

export interface ThreadUsage {
  thread_id: number
  total_prompt_tokens: number
  total_completion_tokens: number
  total_tokens: number
  total_cost_usd: number
  message_count: number
}

export interface SearchMatch {
  message_id: number
  role: string
  snippet: string
  created_at: string
}

export interface SearchResult {
  thread: Thread
  matches: SearchMatch[]
}

// Global search types
export interface GlobalSearchTaskResult {
  id: string
  title: string
  status: TaskStatus
  project_name: string
  updated_at: string
}

export interface GlobalSearchProjectResult {
  id: string
  name: string
  path: string
}

export interface GlobalSearchThreadResult {
  id: number
  title: string
  updated_at: string
}

export interface GlobalSearchMessageResult {
  id: number
  thread_id: number
  thread_title: string
  snippet: string
  created_at: string
}

export interface GlobalSearchResults {
  tasks: GlobalSearchTaskResult[]
  projects: GlobalSearchProjectResult[]
  threads: GlobalSearchThreadResult[]
  messages: GlobalSearchMessageResult[]
}

// Cross-thread message full-text search hits (GET /api/v1/search/messages).
export interface MessageSearchHit {
  message_id: number
  thread_id: number
  thread_title: string
  role: string
  created_at: string
  content_snippet: string
  rank: number
}

export interface MessageSearchResponse {
  data: MessageSearchHit[]
  total: number
}

// ── Box server dashboard ──

export interface BoxServiceStatus {
  name: string
  port: number
  description: string
  type: 'systemd' | 'manual'
  vram_usage_mb: number
  status: 'running' | 'stopped'
  url: string
}

export interface BoxStatus {
  online: boolean
  host: string
  services: BoxServiceStatus[]
}

export interface BoxProject {
  id: string
  name: string
  path: string
}

export interface BoxProjectsResponse {
  data: BoxProject[]
  online: boolean
  note?: string
  cached?: boolean
}

// ── MCP Servers ──

export type MCPServerType = 'stdio' | 'sse'

export interface MCPServerStdioConfig {
  command: string
  args?: string[]
  env?: Record<string, string>
}

export interface MCPServerSSEConfig {
  url: string
  headers?: Record<string, string>
}

export interface MCPServer {
  id: number
  name: string
  server_type: MCPServerType
  config: MCPServerStdioConfig | MCPServerSSEConfig
  is_default: boolean
  active: boolean
  created_at: string
  updated_at: string
}

export interface MCPServerWithStatus {
  id: number
  name: string
  server_type: MCPServerType
  is_default: boolean
  active: boolean
  enabled: boolean
}

// ── Recurring Task Schedules ──

export interface TaskSchedule {
  id: number
  project_id: string
  project?: Project
  title: string
  spec: string
  cron_expression: string
  priority: number
  enabled: boolean
  last_run_at: string | null
  next_run_at: string | null
  created_at: string
  updated_at: string
}

// ── Cron Jobs ──

export type CronExecutionStatus = 'running' | 'success' | 'failed' | 'timeout'

export interface CronJob {
  id: number
  name: string
  schedule: string
  prompt: string
  project_id: string
  project?: Project
  enabled: boolean
  timeout_minutes: number
  model: string | null
  last_run_at: string | null
  last_status: string | null
  created_at: string
  updated_at: string
}

export interface CronExecution {
  id: number
  cron_job_id: number
  status: CronExecutionStatus
  output: string | null
  error_message: string | null
  cost_usd: number
  input_tokens: number
  output_tokens: number
  duration_ms: number
  started_at: string
  finished_at: string | null
}

// Cost analytics types

export interface ModelTokens {
  input: number
  output: number
}

export interface CostByDate {
  date: string
  cost_usd: number
  input_tokens: number
  output_tokens: number
  by_model: Record<string, ModelTokens>
}

export interface CostByModel {
  model: string
  input_tokens: number
  output_tokens: number
  cost_usd: number
  message_count: number
}

export interface CostByThread {
  thread_id: number
  title: string
  cost_usd: number
  input_tokens: number
  output_tokens: number
}

export interface CostByProject {
  project_name: string
  cost_usd: number
  input_tokens: number
  output_tokens: number
}

export interface CostAnalytics {
  total_cost_usd: number
  total_input_tokens: number
  total_output_tokens: number
  by_date: CostByDate[]
  by_model: CostByModel[]
  by_thread: CostByThread[]
  by_project: CostByProject[]
}

// Aggregated task stats from /api/v1/stats/tasks. Each bucket fills in
// exactly one of the keying fields (day, project, or model) depending on the
// requested group_by.
export interface TaskStatsBucket {
  day?: string
  project?: string
  project_id?: string
  model?: string
  task_count: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_creation_tokens: number
  cost_usd: number
}

export type TaskStatsGroupBy = 'day' | 'project' | 'model' | 'day,project' | 'day,model'

export interface TaskStatsAggregated {
  from: string
  to: string
  group_by: TaskStatsGroupBy
  buckets: TaskStatsBucket[]
}

export interface PushSubscriptionInfo {
  id: number
  user_id: number
  endpoint: string
  p256dh: string
  auth: string
  user_agent?: string
  created_at: string
  last_used_at?: string | null
}
