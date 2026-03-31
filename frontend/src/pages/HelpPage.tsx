import {
  Layers,
  MessageSquare,
  FolderGit2,
  Keyboard,
  Zap,
  ListTodo,
  Mic,
  Paperclip,
  Search,
  Palette,
  Terminal,
  BarChart3,
  Settings,
  Globe,
  FileText,
  Gauge,
} from 'lucide-react'

import { useDocumentTitle } from '../hooks/useDocumentTitle'

const contextLayers = [
  {
    num: 1,
    name: 'SOUL.md',
    heading: '# Identity',
    description: 'AI personality and identity definition',
    source: '$OPENCLAW_WORKSPACE/SOUL.md',
    condition: 'Included if file exists',
  },
  {
    num: 2,
    name: 'USER.md',
    heading: '# About the User',
    description: 'Information about the user',
    source: '$OPENCLAW_WORKSPACE/USER.md',
    condition: 'Included if file exists',
  },
  {
    num: 3,
    name: 'TOOLS.md',
    heading: '# Available Tools',
    description: 'Available tools and commands',
    source: '$OPENCLAW_WORKSPACE/TOOLS.md',
    condition: 'Included if file exists',
  },
  {
    num: 4,
    name: 'MEMORY.md',
    heading: '# Operational Memory',
    description: 'Long-term operational memory',
    source: '$OPENCLAW_WORKSPACE/MEMORY.md',
    condition: 'Included if file exists',
  },
  {
    num: 5,
    name: 'Daily Notes',
    heading: '# Recent Notes',
    description: 'Last 3 days of daily memory files, most recent first',
    source: '$OPENCLAW_WORKSPACE/memory/YYYY-MM-DD.md',
    condition: 'Included if files exist',
  },
  {
    num: 6,
    name: 'App Memories',
    heading: '# User Preferences',
    description: 'User-created memories from the Botka database',
    source: 'Settings > Memories',
    condition: 'All active memories concatenated',
  },
  {
    num: 7,
    name: 'System Prompt',
    heading: '# Thread Instructions',
    description: "Thread's persona or custom system prompt",
    source: 'Thread persona / custom prompt',
    condition: 'Included if set on the thread',
  },
  {
    num: 8,
    name: 'Project CLAUDE.md',
    heading: '# Project Context',
    description: 'CLAUDE.md content from the assigned project',
    source: 'Project directory CLAUDE.md',
    condition: 'Only if a project is assigned',
  },
  {
    num: 9,
    name: 'Conversation History',
    heading: '# Previous Conversation',
    description: 'Last 200 messages, each truncated to 500 chars',
    source: 'Thread message history',
    condition: 'Only for new sessions (not resume)',
  },
]

const claudeMdHierarchy = [
  {
    num: 1,
    name: 'Global',
    path: '~/.claude/CLAUDE.md',
    description: "User's private instructions that apply to all projects. Not checked into any repo.",
  },
  {
    num: 2,
    name: 'Project',
    path: '<project>/CLAUDE.md',
    description:
      'Project-specific instructions checked into the repo. Shared with the team via version control.',
  },
  {
    num: 3,
    name: 'Auto-memory',
    path: '~/.claude/projects/<encoded-dir>/memory/MEMORY.md',
    description:
      'Automatic per-project memory. Claude Code writes here to remember context across conversations.',
  },
]

const shortcuts = [
  { keys: 'Ctrl+Shift+O', description: 'New chat' },
  { keys: 'Ctrl+Shift+P', description: 'Pin / unpin thread' },
  { keys: 'Ctrl+Shift+Arrow', description: 'Navigate between threads' },
  { keys: 'Ctrl+K', description: 'Command palette' },
  { keys: '/', description: 'Focus chat input' },
  { keys: 'Ctrl+L', description: 'Focus chat input' },
  { keys: 'Shift+Tab', description: 'Toggle plan/act mode (when textarea focused)' },
  { keys: '?', description: 'Show shortcuts modal' },
  { keys: 'Escape', description: 'Close modals / blur input' },
  { keys: 'Arrow Up/Down', description: 'Browse input history (in empty textarea)' },
]

const slashCommands = [
  { name: '/new', description: 'Start a new thread' },
  { name: '/status', description: 'Show current model info' },
  { name: '/model', description: 'Switch the active model' },
  { name: '/export', description: 'Export thread (md or json)' },
  { name: '/search', description: 'Open the search panel' },
  { name: '/clear', description: "Clear this thread's messages" },
  { name: '/compact', description: 'Compact Claude context' },
  { name: '/reset', description: 'Reset Claude session (fresh start)' },
]

function Section({
  icon: Icon,
  title,
  children,
}: {
  icon: typeof Layers
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="mb-8">
      <h2 className="flex items-center gap-2 text-lg font-semibold text-zinc-900 mb-3">
        <Icon className="h-5 w-5 text-amber-600" />
        {title}
      </h2>
      <div className="text-sm text-zinc-600 leading-relaxed">{children}</div>
    </section>
  )
}

export default function HelpPage() {
  useDocumentTitle('Help')
  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="text-2xl font-bold text-zinc-900 mb-6">Help</h1>

      <Section icon={MessageSquare} title="Chat">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            Each conversation lives in a <strong>thread</strong>. Threads can be pinned, archived,
            tagged, and assigned to projects.
          </li>
          <li>
            When a project is assigned, Claude runs in that project's directory and gets its{' '}
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">CLAUDE.md</code> as context.
          </li>
          <li>
            Switch AI models per-thread via the header dropdown or the{' '}
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">/model</code> command.
          </li>
          <li>
            <strong>Plan / Act mode</strong> — toggle with{' '}
            <kbd className="rounded border border-zinc-300 bg-zinc-100 px-1.5 py-0.5 text-xs font-mono">
              Shift+Tab
            </kbd>{' '}
            in the textarea. Plan mode asks Claude to think before acting.
          </li>
          <li>
            <strong>Export</strong> threads as Markdown or JSON via the thread menu or{' '}
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">/export</code>.
          </li>
        </ul>
      </Section>

      <Section icon={Mic} title="Voice Input">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            Click the microphone icon in the chat input to dictate a message.
          </li>
          <li>
            Uses the browser's Web Speech API when available, with Whisper API as a fallback.
          </li>
          <li>Recording times out after 2 minutes.</li>
        </ul>
      </Section>

      <Section icon={Paperclip} title="File Uploads">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            Attach files by clicking the paperclip icon or pasting from clipboard.
          </li>
          <li>
            Supported types: images, PDFs, text, Markdown, calendar files, and ZIPs.
          </li>
          <li>Maximum file size: 10 MB per file.</li>
        </ul>
      </Section>

      <Section icon={Globe} title="URL Sources">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            Add URLs to a thread to include fetched web content as context.
          </li>
          <li>
            Manage URL sources from the thread menu. Sources are fetched into context when a session starts.
          </li>
        </ul>
      </Section>

      <Section icon={Terminal} title="Slash Commands">
        <p className="mb-3">
          Type{' '}
          <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">/</code> in the chat input
          to see available commands:
        </p>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr>
                <th className="text-left font-medium border-b border-zinc-200 px-3 py-2 bg-zinc-100">
                  Command
                </th>
                <th className="text-left font-medium border-b border-zinc-200 px-3 py-2 bg-zinc-100">
                  Action
                </th>
              </tr>
            </thead>
            <tbody>
              {slashCommands.map((c) => (
                <tr key={c.name}>
                  <td className="border-b border-zinc-200/50 px-3 py-2">
                    <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs font-mono text-emerald-600">
                      {c.name}
                    </code>
                  </td>
                  <td className="border-b border-zinc-200/50 px-3 py-2">{c.description}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Section>

      <Section icon={Search} title="Search & Command Palette">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            <kbd className="rounded border border-zinc-300 bg-zinc-100 px-1.5 py-0.5 text-xs font-mono">
              Ctrl+K
            </kbd>{' '}
            opens the command palette — search threads, tasks, projects, messages, and slash commands.
          </li>
          <li>
            The sidebar search box filters threads by title and searches message content.
          </li>
        </ul>
      </Section>

      <Section icon={Palette} title="Personas & Tags">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            <strong>Personas</strong> — custom AI personalities with a system prompt, default model,
            icon, and optional starter message. Create them in Settings &gt; Personas.
          </li>
          <li>
            Selecting a persona when starting a new thread sets the system prompt and model. The
            starter message is sent automatically.
          </li>
          <li>
            <strong>Tags</strong> — colorable labels for organizing threads. Create them in Settings
            &gt; Tags, then apply via the thread menu.
          </li>
          <li>Filter the thread list by tag or project using the sidebar filters.</li>
        </ul>
      </Section>

      <Section icon={FileText} title="CLAUDE.md Instruction Files">
        <p className="mb-3">
          Claude Code loads instruction files in a specific hierarchy. Each level narrows the scope
          of the instructions:
        </p>

        <div className="overflow-x-auto mb-4">
          <table className="w-full text-sm">
            <thead>
              <tr>
                <th className="text-left font-medium border-b border-zinc-200 px-3 py-2 bg-zinc-100">
                  #
                </th>
                <th className="text-left font-medium border-b border-zinc-200 px-3 py-2 bg-zinc-100">
                  Scope
                </th>
                <th className="text-left font-medium border-b border-zinc-200 px-3 py-2 bg-zinc-100">
                  Path
                </th>
              </tr>
            </thead>
            <tbody>
              {claudeMdHierarchy.map((item) => (
                <tr key={item.num}>
                  <td className="border-b border-zinc-200/50 px-3 py-2 text-amber-600 font-semibold">
                    {item.num}
                  </td>
                  <td className="border-b border-zinc-200/50 px-3 py-2">
                    <div className="font-medium text-zinc-900">{item.name}</div>
                    <div className="text-xs text-zinc-400">{item.description}</div>
                  </td>
                  <td className="border-b border-zinc-200/50 px-3 py-2">
                    <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">
                      {item.path}
                    </code>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="rounded-lg border border-zinc-200 bg-zinc-100/50 p-3 space-y-1.5 text-xs text-zinc-500">
          <p>
            All three levels are loaded into every Claude Code conversation. If instructions
            conflict, more specific levels (project) override broader ones (global).
          </p>
          <p>
            <strong className="text-zinc-700">Botka's context assembly</strong> (below) adds
            additional layers on top of this hierarchy when running chat sessions through this app.
          </p>
        </div>
      </Section>

      <Section icon={Layers} title="Context Assembly">
        <p className="mb-3">
          When a new Claude session starts, Botka assembles a 9-layer hierarchical context and
          passes it via{' '}
          <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">
            --append-system-prompt-file
          </code>
          . Each layer provides progressively narrower context:
        </p>

        <div className="overflow-x-auto mb-4">
          <table className="w-full text-sm">
            <thead>
              <tr>
                <th className="text-left font-medium border-b border-zinc-200 px-3 py-2 bg-zinc-100">
                  #
                </th>
                <th className="text-left font-medium border-b border-zinc-200 px-3 py-2 bg-zinc-100">
                  Layer
                </th>
                <th className="text-left font-medium border-b border-zinc-200 px-3 py-2 bg-zinc-100">
                  Source
                </th>
                <th className="text-left font-medium border-b border-zinc-200 px-3 py-2 bg-zinc-100">
                  Included
                </th>
              </tr>
            </thead>
            <tbody>
              {contextLayers.map((layer) => (
                <tr key={layer.num}>
                  <td className="border-b border-zinc-200/50 px-3 py-2 text-amber-600 font-semibold">
                    {layer.num}
                  </td>
                  <td className="border-b border-zinc-200/50 px-3 py-2">
                    <div className="font-medium text-zinc-900">{layer.name}</div>
                    <div className="text-xs text-zinc-400">{layer.description}</div>
                  </td>
                  <td className="border-b border-zinc-200/50 px-3 py-2">
                    <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">
                      {layer.source}
                    </code>
                  </td>
                  <td className="border-b border-zinc-200/50 px-3 py-2 text-xs text-zinc-500">
                    {layer.condition}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="rounded-lg border border-zinc-200 bg-zinc-100/50 p-3 space-y-1.5 text-xs text-zinc-500">
          <p>
            Context is only assembled for <strong className="text-zinc-700">new sessions</strong>{' '}
            (when there is no existing Claude session ID). Resumed sessions via{' '}
            <code className="rounded bg-zinc-100 px-1 py-0.5">--resume</code> already have their
            context.
          </p>
          <p>
            <strong className="text-zinc-700">App memories</strong> (layer 6) are managed in
            Settings &gt; Memories and persist across all threads.
          </p>
        </div>
      </Section>

      <Section icon={Zap} title="Session Lifecycle">
        <ol className="space-y-2 list-decimal pl-5">
          <li>
            <strong>New thread</strong> — creates a new Claude session with full context assembled.
          </li>
          <li>
            <strong>Subsequent messages</strong> — uses{' '}
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">--resume</code> with the
            existing session ID. No context reassembly.
          </li>
          <li>
            <strong>Session clear</strong> (via{' '}
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">/reset</code> or thread
            menu) — resets the session ID. The next message creates a new session with full context.
          </li>
          <li>
            <strong>Session pool</strong> — a warm Claude process is kept ready between messages for
            fast response times. Evicted on model or project changes.
          </li>
        </ol>
      </Section>

      <Section icon={ListTodo} title="Tasks">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            Tasks are autonomous Claude Code sessions that run in the background without
            interaction.
          </li>
          <li>
            Create tasks from the Tasks page or via the MCP tools (
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">create_task</code>).
          </li>
          <li>
            Each task is assigned to a project and runs in that project's directory with its branch
            strategy.
          </li>
          <li>
            Statuses: Pending, Queued, Running, Done, Failed, Needs Review, Cancelled.
          </li>
          <li>Task output streams live via SSE — click a running task to watch.</li>
          <li>Tasks have retry logic with backoff and optional verification commands.</li>
          <li>
            The <strong>task runner</strong> is controlled from Settings &gt; Task Runner (start,
            pause, stop, set worker count).
          </li>
        </ul>
      </Section>

      <Section icon={Gauge} title="Scheduler & Rate Limits">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            The task scheduler automatically pauses when Anthropic API usage approaches rate limits.
            Chat sessions are <strong>not affected</strong> — only autonomous task execution is paused.
          </li>
          <li>
            <strong>5-hour window</strong> — the scheduler stops picking new tasks when utilization
            exceeds <strong>90%</strong> (
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">USAGE_THRESHOLD_5H = 0.90</code>
            ).
          </li>
          <li>
            <strong>7-day window</strong> — the scheduler stops picking new tasks when utilization
            exceeds <strong>95%</strong> (
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">USAGE_THRESHOLD_7D = 0.95</code>
            ).
          </li>
          <li>
            Usage is checked every 30 seconds via the{' '}
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">claude-usage</code> command.
          </li>
          <li>
            When the rate limit window resets or utilization drops below the threshold, the scheduler
            automatically resumes picking tasks.
          </li>
        </ul>
      </Section>

      <Section icon={BarChart3} title="Cost & Usage">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            The <strong>Cost</strong> page shows token usage over time, broken down by model.
          </li>
          <li>See top threads and projects by cost for a selected period (7d, 30d, 90d).</li>
          <li>
            The <strong>Dashboard</strong> shows task stats (success rate, average duration, total
            cost) and API rate limit usage for the 5-hour and 7-day windows.
          </li>
        </ul>
      </Section>

      <Section icon={FolderGit2} title="Projects">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            Projects are git repositories auto-discovered from the{' '}
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">PROJECTS_DIR</code>{' '}
            directory.
          </li>
          <li>
            Each project can have a <strong>branch strategy</strong> (for tasks) and a{' '}
            <strong>verification command</strong>.
          </li>
          <li>
            Project detail pages show task stats, git status, commit history, and related tasks.
          </li>
          <li>
            Assigning a project to a thread sets the working directory and injects the project's
            CLAUDE.md into context.
          </li>
        </ul>
      </Section>

      <Section icon={Settings} title="Settings">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            <strong>General</strong> — theme (light, dark, dark green, dark blue), font size,
            default model, notification sound, send-on-enter toggle.
          </li>
          <li>
            <strong>Security</strong> — change password, manage passkeys (WebAuthn).
          </li>
          <li>
            <strong>Users</strong> — create users, assign roles (Admin or External). External users
            have read-only chat access.
          </li>
          <li>
            <strong>Task Runner</strong> — start/pause/stop the runner, set max concurrent workers.
          </li>
          <li>
            <strong>Personas</strong> — create AI personalities with custom prompts, models, icons,
            and starter messages.
          </li>
          <li>
            <strong>Tags</strong> — create colorable labels for organizing threads.
          </li>
          <li>
            <strong>Memories</strong> — store persistent notes that are included in every chat
            session (context layer 5).
          </li>
          <li>
            <strong>Voice</strong> — voice input and transcription settings.
          </li>
        </ul>
      </Section>

      <Section icon={Keyboard} title="Keyboard Shortcuts">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr>
                <th className="text-left font-medium border-b border-zinc-200 px-3 py-2 bg-zinc-100">
                  Shortcut
                </th>
                <th className="text-left font-medium border-b border-zinc-200 px-3 py-2 bg-zinc-100">
                  Action
                </th>
              </tr>
            </thead>
            <tbody>
              {shortcuts.map((s) => (
                <tr key={s.keys}>
                  <td className="border-b border-zinc-200/50 px-3 py-2">
                    <kbd className="rounded border border-zinc-300 bg-zinc-100 px-2 py-0.5 text-xs font-mono text-zinc-700">
                      {s.keys}
                    </kbd>
                  </td>
                  <td className="border-b border-zinc-200/50 px-3 py-2">{s.description}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Section>

      <div className="rounded-lg border border-zinc-200 bg-zinc-100/50 p-4 text-xs text-zinc-500 mb-8">
        <p>
          <strong className="text-zinc-700">PWA</strong> — Botka can be installed as an app from your
          browser. An update banner appears when a new version is available.
        </p>
      </div>
    </div>
  )
}
