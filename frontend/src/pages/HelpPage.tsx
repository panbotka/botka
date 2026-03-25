import {
  Layers,
  MessageSquare,
  FolderGit2,
  Keyboard,
  Zap,
  ListTodo,
} from 'lucide-react'

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
    name: 'MEMORY.md',
    heading: '# Operational Memory',
    description: 'Long-term operational memory',
    source: '$OPENCLAW_WORKSPACE/MEMORY.md',
    condition: 'Included if file exists',
  },
  {
    num: 4,
    name: 'Daily Notes',
    heading: '# Recent Notes',
    description: 'Last 3 days of daily memory files, most recent first',
    source: '$OPENCLAW_WORKSPACE/memory/YYYY-MM-DD.md',
    condition: 'Included if files exist',
  },
  {
    num: 5,
    name: 'App Memories',
    heading: '# User Preferences',
    description: 'User-created memories from the Botka database',
    source: 'Settings > Memories',
    condition: 'All active memories concatenated',
  },
  {
    num: 6,
    name: 'System Prompt',
    heading: '# Thread Instructions',
    description: "Thread's persona or custom system prompt",
    source: 'Thread persona / custom prompt',
    condition: 'Included if set on the thread',
  },
  {
    num: 7,
    name: 'Project CLAUDE.md',
    heading: '# Project Context',
    description: 'CLAUDE.md content from the assigned project',
    source: 'Project directory CLAUDE.md',
    condition: 'Only if a project is assigned',
  },
  {
    num: 8,
    name: 'Conversation History',
    heading: '# Previous Conversation',
    description: 'Last 200 messages, each truncated to 500 chars',
    source: 'Thread message history',
    condition: 'Only for new sessions (not resume)',
  },
]

const shortcuts = [
  { keys: 'Ctrl+Shift+O', description: 'New chat' },
  { keys: 'Ctrl+Shift+P', description: 'Pin / unpin thread' },
  { keys: 'Ctrl+Shift+Arrow', description: 'Navigate between threads' },
  { keys: 'Ctrl+K', description: 'Command palette' },
  { keys: '/', description: 'Focus chat input' },
  { keys: 'Ctrl+L', description: 'Focus chat input' },
  { keys: 'Shift+Tab', description: 'Toggle plan mode (when textarea focused)' },
  { keys: '?', description: 'Show shortcuts modal' },
  { keys: 'Escape', description: 'Close modals / blur input' },
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
  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="text-2xl font-bold text-zinc-900 mb-6">Help</h1>

      <Section icon={MessageSquare} title="Chat & Projects">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            Threads (chats) can optionally be assigned to a <strong>project</strong>.
          </li>
          <li>
            When a project is assigned, Claude runs in that project's directory as its working
            directory.
          </li>
          <li>
            When no project is assigned, Claude runs in the default directory (
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">CLAUDE_DEFAULT_WORK_DIR</code>
            ).
          </li>
          <li>
            The project's <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">CLAUDE.md</code>{' '}
            content is injected into the context (layer 7).
          </li>
          <li>
            Projects are git repositories auto-discovered from the{' '}
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">PROJECTS_DIR</code>{' '}
            directory.
          </li>
          <li>The sidebar shows a project badge on threads that have a project assigned.</li>
        </ul>
      </Section>

      <Section icon={Layers} title="Context Assembly">
        <p className="mb-3">
          When a new Claude session starts, Botka assembles an 8-layer hierarchical context and
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
            The <strong className="text-zinc-700">session pool</strong> pre-warms Claude processes
            between messages — the next message reuses a warm process, skipping startup time.
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
            <strong>Session clear</strong> (from UI) — resets the session ID. The next message
            creates a new session with full context.
          </li>
          <li>
            <strong>Session pool</strong> — keeps a warm process ready for 5 minutes after each
            message for fast response times.
          </li>
        </ol>
      </Section>

      <Section icon={ListTodo} title="Task Execution">
        <p className="mb-2">Tasks differ from chat in several ways:</p>
        <ul className="space-y-2 list-disc pl-5">
          <li>
            Tasks run <strong>standalone</strong> Claude sessions — no{' '}
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">--resume</code>.
          </li>
          <li>
            Tasks run in the project's directory using the project's branch strategy.
          </li>
          <li>Tasks have retry logic with backoff and optional verification commands.</li>
          <li>Task output is streamed live via SSE.</li>
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

      <Section icon={FolderGit2} title="Projects">
        <ul className="space-y-2 list-disc pl-5">
          <li>
            Projects are git repositories discovered from the{' '}
            <code className="rounded bg-zinc-100 px-1.5 py-0.5 text-xs">PROJECTS_DIR</code>{' '}
            directory.
          </li>
          <li>
            Each project can have a <strong>branch strategy</strong> (for tasks) and a{' '}
            <strong>verification command</strong>.
          </li>
          <li>
            Assigning a project to a thread sets the working directory and injects the project's
            CLAUDE.md into context.
          </li>
        </ul>
      </Section>
    </div>
  )
}
