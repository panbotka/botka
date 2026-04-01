import { useState, useEffect, useRef } from 'react'
import { FileText } from 'lucide-react'
import { updateCustomContext } from '../api/client'

interface Props {
  threadId: number
  initialContent: string
  onClose: () => void
  onSaved: () => void
}

export default function CustomContextEditor({ threadId, initialContent, onClose, onSaved }: Props) {
  const [content, setContent] = useState(initialContent)
  const [saving, setSaving] = useState(false)
  const [dirty, setDirty] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    textareaRef.current?.focus()
  }, [])

  const handleSave = async () => {
    if (!dirty) return
    setSaving(true)
    try {
      await updateCustomContext(threadId, content)
      setDirty(false)
      onSaved()
    } finally {
      setSaving(false)
    }
  }

  const handleClose = async () => {
    if (dirty) await handleSave()
    onClose()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
         onClick={handleClose}>
      <div className="bg-white dark:bg-zinc-100 rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[80vh] flex flex-col"
           onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 py-4 border-b border-zinc-100">
          <div className="flex items-center gap-2">
            <FileText className="w-4 h-4 text-zinc-500" />
            <h2 className="text-sm font-semibold text-zinc-800">Custom Context</h2>
          </div>
          <button onClick={handleClose}
                  className="text-xs text-zinc-400 hover:text-zinc-600 cursor-pointer">
            Done
          </button>
        </div>

        <div className="px-5 py-3 flex-1 flex flex-col min-h-0">
          <p className="text-xs text-zinc-400 mb-2">
            Reference material included in every message. Paste API docs, schemas, notes, etc.
          </p>
          <textarea
            ref={textareaRef}
            value={content}
            onChange={e => { setContent(e.target.value); setDirty(true) }}
            onBlur={handleSave}
            rows={12}
            placeholder="Paste reference material here..."
            className="flex-1 min-h-[200px] w-full text-sm bg-zinc-50 border border-zinc-200
                       rounded-lg px-3 py-2 text-zinc-800 placeholder-zinc-300
                       focus:border-zinc-400 focus:outline-none resize-y
                       font-mono leading-relaxed"
          />
        </div>

        <div className="px-5 py-3 border-t border-zinc-100 flex items-center justify-between">
          <span className="text-[11px] text-zinc-400">
            {content.length.toLocaleString()} chars
          </span>
          {saving && <span className="text-[11px] text-zinc-400">Saving...</span>}
        </div>
      </div>
    </div>
  )
}
