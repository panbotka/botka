import { useEffect, useState } from 'react'
import {
  X,
  Cpu,
  Palette,
  Globe,
  FileText,
  MessageSquare,
  Plug,
  ChevronRight,
  User,
} from 'lucide-react'
import type { Tag, Thread } from '../types'
import { api } from '../api/client'
import { THREAD_COLORS } from '../utils/threadColors'
import ModelPicker from './ModelPicker'
import ThreadSourcesEditor from './ThreadSourcesEditor'
import CustomContextEditor from './CustomContextEditor'
import SignalBridgeEditor from './SignalBridgeEditor'
import MCPServerToggle from './MCPServerToggle'

interface Props {
  thread: Thread
  tags: Tag[]
  onClose: () => void
  onThreadsChange: () => void
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div className="px-1 pb-1.5 text-[11px] font-medium text-zinc-400 uppercase tracking-wider">
      {children}
    </div>
  )
}

function Section({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="mb-4">
      <SectionLabel>{label}</SectionLabel>
      <div className="bg-white border border-zinc-200 rounded-xl p-3">
        {children}
      </div>
    </div>
  )
}

export default function ThreadSettingsPanel({
  thread,
  tags,
  onClose,
  onThreadsChange,
}: Props) {
  const [sourcesOpen, setSourcesOpen] = useState(false)
  const [customContextOpen, setCustomContextOpen] = useState(false)
  const [signalBridgeOpen, setSignalBridgeOpen] = useState(false)
  const [mcpServersOpen, setMcpServersOpen] = useState(false)

  // Close on Escape — but not when a nested modal is open (let the modal handle it)
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      if (sourcesOpen || customContextOpen || signalBridgeOpen || mcpServersOpen) return
      onClose()
    }
    document.addEventListener('keydown', handleKey)
    return () => document.removeEventListener('keydown', handleKey)
  }, [onClose, sourcesOpen, customContextOpen, signalBridgeOpen, mcpServersOpen])

  const handleChangeModel = async (model: string) => {
    try {
      await api.updateModel(thread.id, model)
      onThreadsChange()
    } catch { /* ignore */ }
  }

  const handleChangeColor = async (color: string) => {
    try {
      await api.updateThread(thread.id, { color } as Partial<Thread>)
      onThreadsChange()
    } catch { /* ignore */ }
  }

  const handleToggleTag = async (tagId: number) => {
    const currentTagIds = (thread.tags || []).map(t => t.id)
    const newTagIds = currentTagIds.includes(tagId)
      ? currentTagIds.filter(id => id !== tagId)
      : [...currentTagIds, tagId]
    try {
      await api.updateThreadTags(thread.id, newTagIds)
      onThreadsChange()
    } catch { /* ignore */ }
  }

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 z-40 bg-black/20"
        onClick={onClose}
        data-testid="thread-settings-backdrop"
      />

      {/* Panel */}
      <aside
        className="fixed right-0 top-0 bottom-0 z-50 w-full md:w-[380px]
                   bg-zinc-50 border-l border-zinc-200 shadow-xl
                   flex flex-col
                   animate-in slide-in-from-right duration-200"
        role="dialog"
        aria-label="Thread settings"
      >
        {/* Header */}
        <header className="flex items-center gap-2 px-4 h-12 border-b border-zinc-200 bg-zinc-50 flex-shrink-0">
          <div className="flex-1 min-w-0">
            <span className="text-sm font-medium text-zinc-900 truncate block">
              {thread.persona_icon && <span className="mr-1">{thread.persona_icon}</span>}
              {thread.title || 'New conversation'}
            </span>
          </div>
          <button
            onClick={onClose}
            aria-label="Close settings"
            className="text-zinc-400 hover:text-zinc-700 transition-colors cursor-pointer
                       w-8 h-8 flex items-center justify-center rounded-lg hover:bg-zinc-100"
          >
            <X className="w-4 h-4" />
          </button>
        </header>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-4">
          {/* Persona */}
          <Section label="Persona">
            <div className="flex items-center gap-2">
              {thread.persona_icon ? (
                <span className="text-base flex-shrink-0">{thread.persona_icon}</span>
              ) : (
                <User className="w-4 h-4 text-zinc-400 flex-shrink-0" />
              )}
              <span className="text-sm text-zinc-700 truncate">
                {thread.persona_name || 'No persona'}
              </span>
            </div>
          </Section>

          {/* Model */}
          <Section label="Model">
            <div className="flex items-center gap-2 mb-2 text-zinc-500 text-xs">
              <Cpu className="w-3.5 h-3.5" />
              <span>Current: {thread.model || 'Default'}</span>
            </div>
            <ModelPicker
              value={thread.model}
              onChange={handleChangeModel}
            />
          </Section>

          {/* Color */}
          <Section label="Color">
            <div className="flex items-center gap-2 mb-2 text-zinc-500 text-xs">
              <Palette className="w-3.5 h-3.5" />
              <span>Sidebar accent</span>
            </div>
            <div className="grid grid-cols-4 gap-2">
              {THREAD_COLORS.map(color => {
                const isActive = (thread.color || '') === color.key
                return (
                  <button
                    key={color.key || 'none'}
                    onClick={() => handleChangeColor(color.key)}
                    className={`w-full aspect-square rounded-lg border-2 transition-all cursor-pointer flex items-center justify-center
                      ${isActive ? 'border-zinc-500 scale-105' : 'border-zinc-200 hover:border-zinc-400'}`}
                    style={{ backgroundColor: color.key ? color.swatch : undefined }}
                    title={color.label}
                    aria-label={color.label}
                    aria-pressed={isActive}
                  >
                    {color.key === '' && (
                      <X className="w-4 h-4 text-zinc-400" />
                    )}
                  </button>
                )
              })}
            </div>
          </Section>

          {/* Tags */}
          <Section label="Tags">
            {tags.length === 0 ? (
              <p className="text-xs text-zinc-400">No tags created yet.</p>
            ) : (
              <div className="flex flex-col gap-1">
                {tags.map(tag => {
                  const isActive = (thread.tags || []).some(t => t.id === tag.id)
                  return (
                    <button
                      key={tag.id}
                      onClick={() => handleToggleTag(tag.id)}
                      className="w-full flex items-center gap-2.5 px-2 py-1.5
                                 text-sm text-zinc-700 hover:bg-zinc-50
                                 transition-colors cursor-pointer rounded-lg"
                      aria-pressed={isActive}
                    >
                      <span
                        className="w-3.5 h-3.5 rounded-full flex-shrink-0 border-2"
                        style={{
                          backgroundColor: isActive ? tag.color : 'transparent',
                          borderColor: tag.color,
                        }}
                      />
                      <span className="truncate flex-1 text-left">{tag.name}</span>
                    </button>
                  )
                })}
              </div>
            )}
          </Section>

          {/* Sources */}
          <Section label="Sources">
            <button
              onClick={() => setSourcesOpen(true)}
              className="w-full flex items-center gap-3 px-2 py-1.5
                         text-sm text-zinc-700 hover:bg-zinc-50
                         transition-colors cursor-pointer rounded-lg"
            >
              <Globe className="w-4 h-4 flex-shrink-0 text-zinc-400" />
              <span className="flex-1 text-left">Manage sources</span>
              <ChevronRight className="w-3.5 h-3.5 text-zinc-400 flex-shrink-0" />
            </button>
          </Section>

          {/* Custom Context */}
          <Section label="Custom Context">
            <button
              onClick={() => setCustomContextOpen(true)}
              className="w-full flex items-center gap-3 px-2 py-1.5
                         text-sm text-zinc-700 hover:bg-zinc-50
                         transition-colors cursor-pointer rounded-lg"
            >
              <FileText className="w-4 h-4 flex-shrink-0 text-zinc-400" />
              <span className="flex-1 text-left">Edit custom context</span>
              {thread.custom_context && thread.custom_context.length > 0 && (
                <span className="text-[11px] text-zinc-500 bg-zinc-100 px-2 py-0.5 rounded-md flex-shrink-0">
                  {thread.custom_context.length.toLocaleString()} chars
                </span>
              )}
              <ChevronRight className="w-3.5 h-3.5 text-zinc-400 flex-shrink-0" />
            </button>
          </Section>

          {/* Signal Bridge */}
          <Section label="Signal Bridge">
            <button
              onClick={() => setSignalBridgeOpen(true)}
              className="w-full flex items-center gap-3 px-2 py-1.5
                         text-sm text-zinc-700 hover:bg-zinc-50
                         transition-colors cursor-pointer rounded-lg"
            >
              <MessageSquare
                className={`w-4 h-4 flex-shrink-0 ${
                  thread.signal_bridge_active ? 'text-emerald-500' : 'text-zinc-400'
                }`}
              />
              <span className="flex-1 text-left">Configure Signal Bridge</span>
              {thread.signal_bridge_active && (
                <span className="text-[10px] font-medium text-emerald-700 bg-emerald-50 border border-emerald-200 px-1.5 py-0.5 rounded-md flex-shrink-0">
                  Active
                </span>
              )}
              <ChevronRight className="w-3.5 h-3.5 text-zinc-400 flex-shrink-0" />
            </button>
          </Section>

          {/* MCP Servers */}
          <Section label="MCP Servers">
            <button
              onClick={() => setMcpServersOpen(true)}
              className="w-full flex items-center gap-3 px-2 py-1.5
                         text-sm text-zinc-700 hover:bg-zinc-50
                         transition-colors cursor-pointer rounded-lg"
            >
              <Plug className="w-4 h-4 flex-shrink-0 text-zinc-400" />
              <span className="flex-1 text-left">Manage MCP servers</span>
              <ChevronRight className="w-3.5 h-3.5 text-zinc-400 flex-shrink-0" />
            </button>
          </Section>
        </div>
      </aside>

      {/* Nested modals */}
      {sourcesOpen && (
        <ThreadSourcesEditor
          threadId={thread.id}
          onClose={() => setSourcesOpen(false)}
        />
      )}
      {customContextOpen && (
        <CustomContextEditor
          threadId={thread.id}
          initialContent={thread.custom_context || ''}
          onClose={() => setCustomContextOpen(false)}
          onSaved={onThreadsChange}
        />
      )}
      {signalBridgeOpen && (
        <SignalBridgeEditor
          threadId={thread.id}
          onClose={() => setSignalBridgeOpen(false)}
          onChange={onThreadsChange}
        />
      )}
      {mcpServersOpen && (
        <MCPServerToggle
          scope={{ type: 'thread', id: thread.id }}
          onClose={() => setMcpServersOpen(false)}
        />
      )}
    </>
  )
}
