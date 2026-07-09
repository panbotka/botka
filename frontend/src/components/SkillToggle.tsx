import { Link } from 'react-router-dom'
import { Sparkles, Settings } from 'lucide-react'
import { useThreadSkills } from '../hooks/useThreadSkills'

interface Props {
  threadId: number
  onClose?: () => void
}

/** Renders a skill's origin: user, project, or the owning plugin. */
function SourceBadge({ source }: { source: string }) {
  const isPlugin = source.startsWith('plugin:')
  const label = isPlugin ? source.slice('plugin:'.length) : source
  const tone = isPlugin
    ? 'bg-purple-100 text-purple-700'
    : source === 'project'
      ? 'bg-blue-100 text-blue-700'
      : 'bg-zinc-100 text-zinc-600'
  return (
    <span className={`inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium ${tone}`}>
      {label}
    </span>
  )
}

/**
 * Per-chat skill switches. A skill turned off here is denied for the thread's
 * next message; other chats are unaffected.
 */
export default function SkillToggle({ threadId, onClose }: Props) {
  const { skills, loading, error, toggle } = useThreadSkills(threadId)

  const content = (
    <div className="space-y-1">
      {loading ? (
        <p className="text-xs text-zinc-400 py-4 text-center">Loading...</p>
      ) : error ? (
        <p className="text-xs text-red-500 py-2 text-center">{error}</p>
      ) : skills.length === 0 ? (
        <div className="py-4 text-center">
          <p className="text-xs text-zinc-400">No skills found</p>
          <Link
            to="/settings?tab=skills"
            className="inline-flex items-center gap-1 mt-2 text-xs text-zinc-500 hover:text-zinc-700 transition-colors"
          >
            <Settings className="w-3 h-3" />
            Manage in Settings
          </Link>
        </div>
      ) : (
        <>
          {skills.map((skill) => (
            <div key={skill.name} className="flex items-center gap-2.5 py-1.5 px-1 rounded-lg">
              <button
                onClick={() => toggle(skill.name)}
                aria-label={`Toggle ${skill.name}`}
                className={`relative w-8 h-[18px] rounded-full transition-colors flex-shrink-0 cursor-pointer ${
                  skill.enabled ? 'bg-emerald-500' : 'bg-zinc-300'
                }`}
              >
                <span
                  className={`absolute top-0.5 left-0.5 w-3.5 h-3.5 rounded-full bg-white shadow transition-transform ${
                    skill.enabled ? 'translate-x-3.5' : 'translate-x-0'
                  }`}
                />
              </button>
              <span className="text-sm text-zinc-700 truncate flex-1 min-w-0" title={skill.description}>
                {skill.name}
              </span>
              <SourceBadge source={skill.source} />
            </div>
          ))}
          <p className="text-[10px] text-zinc-400 mt-2 px-1">Changes take effect on the next message</p>
        </>
      )}
    </div>
  )

  if (onClose) {
    return (
      <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={onClose}>
        <div
          className="bg-white dark:bg-zinc-100 rounded-xl shadow-xl w-full max-w-sm mx-4 max-h-[80vh] flex flex-col"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between px-5 py-4 border-b border-zinc-100">
            <div className="flex items-center gap-2">
              <Sparkles className="w-4 h-4 text-zinc-500" />
              <h2 className="text-sm font-semibold text-zinc-800">Skills</h2>
            </div>
            <button onClick={onClose} className="text-xs text-zinc-400 hover:text-zinc-600 cursor-pointer">
              Done
            </button>
          </div>
          <div className="flex-1 overflow-y-auto px-5 py-3">{content}</div>
        </div>
      </div>
    )
  }

  return content
}
