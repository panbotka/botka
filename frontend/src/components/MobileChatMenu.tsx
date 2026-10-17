import { useEffect, useState, type ReactNode } from 'react'
import {
  ArrowLeft,
  Bookmark,
  ChevronRight,
  Command,
  FolderOpen,
  Plus,
  Search,
  Server,
  Settings,
  Trash2,
} from 'lucide-react'
import { clsx } from 'clsx'
import type { Project } from '../types'
import BottomSheet from './BottomSheet'
import BookmarksBar from './BookmarksBar'
import CommandButtons from './CommandButtons'
import { ProjectPickerList, isBoxProject } from './ProjectPicker'

type View = 'main' | 'project' | 'bookmarks'

interface Props {
  open: boolean
  onClose: () => void
  /** All projects, for the project sub-view. */
  projects: Project[]
  /** The thread's current project, if any (drives the Commands section). */
  project: Project | undefined
  onSelectProject: (projectId: string | null) => void
  onBoxProjectsChanged?: () => void
  onOpenPalette: () => void
  onOpenSettings: () => void
  onNewChat: () => void
  onSearchInThread: () => void
  /** Called only after the in-sheet confirmation tap. */
  onClearHistory: () => void
  /** External (read-only) users get no settings, new chat or clear history. */
  readOnly?: boolean
}

function Row({
  icon,
  label,
  detail,
  onClick,
  chevron = false,
  danger = false,
}: {
  icon: ReactNode
  label: string
  detail?: string
  onClick: () => void
  chevron?: boolean
  danger?: boolean
}) {
  return (
    <li>
      <button
        type="button"
        onClick={onClick}
        className={clsx(
          'flex w-full min-h-[48px] items-center gap-3 rounded-lg px-4 text-left text-base font-medium transition-colors cursor-pointer',
          danger ? 'text-red-600 active:bg-red-50' : 'text-zinc-700 active:bg-zinc-100',
        )}
      >
        <span className="flex h-5 w-5 flex-shrink-0 items-center justify-center">{icon}</span>
        <span className="flex-1 truncate">{label}</span>
        {detail && <span className="max-w-[45%] truncate text-sm font-normal text-zinc-400">{detail}</span>}
        {chevron && <ChevronRight className="h-4 w-4 flex-shrink-0 text-zinc-400" />}
      </button>
    </li>
  )
}

/**
 * MobileChatMenu is the "⋯" bottom sheet of the mobile chat header. It collects
 * everything that used to crowd the header row and the composer chips: project,
 * project commands, command palette, bookmarks, thread settings, new chat,
 * in-thread search and clearing the history. Project and bookmarks open as
 * sub-views inside the same sheet. Clearing asks for a second tap first.
 */
export default function MobileChatMenu({
  open,
  onClose,
  projects,
  project,
  onSelectProject,
  onBoxProjectsChanged,
  onOpenPalette,
  onOpenSettings,
  onNewChat,
  onSearchInThread,
  onClearHistory,
  readOnly = false,
}: Props) {
  const [view, setView] = useState<View>('main')
  const [confirmClear, setConfirmClear] = useState(false)

  // Every open starts on the main menu with no pending confirmation.
  useEffect(() => {
    if (open) {
      setView('main')
      setConfirmClear(false)
    }
  }, [open])

  // Run an action that leaves the sheet.
  const run = (action: () => void) => () => {
    onClose()
    action()
  }

  const backHeader = (title: string) => (
    <div className="flex items-center gap-1 px-2 pb-1">
      <button
        type="button"
        onClick={() => setView('main')}
        aria-label="Zpět"
        className="flex min-h-[44px] min-w-[44px] items-center justify-center rounded-lg text-zinc-500 active:bg-zinc-100"
      >
        <ArrowLeft className="h-5 w-5" />
      </button>
      <span className="text-base font-medium text-zinc-900">{title}</span>
    </div>
  )

  let header: ReactNode = null
  let content: ReactNode
  if (view === 'project') {
    header = backHeader('Projekt')
    content = (
      <div className="pb-2">
        <ProjectPickerList
          projects={projects}
          currentProjectId={project?.id}
          onSelect={(projectId) => {
            onSelectProject(projectId)
            onClose()
          }}
          onBoxProjectsChanged={onBoxProjectsChanged}
          autoFocus={false}
          touch
          listClassName=""
        />
      </div>
    )
  } else if (view === 'bookmarks') {
    header = backHeader('Záložky')
    content = <BookmarksBar variant="list" readOnly={readOnly} />
  } else {
    content = (
      <>
        <ul className="p-2 pb-0">
          <Row
            icon={project && isBoxProject(project) ? <Server className="h-5 w-5" /> : <FolderOpen className="h-5 w-5" />}
            label="Projekt"
            detail={project?.name || 'Bez projektu'}
            onClick={() => setView('project')}
            chevron
          />
        </ul>
        <CommandButtons project={project} variant="sheet" />
        <ul className="p-2 pt-0">
          <Row icon={<Command className="h-5 w-5" />} label="Paleta příkazů" onClick={run(onOpenPalette)} />
          <Row icon={<Bookmark className="h-5 w-5" />} label="Záložky" onClick={() => setView('bookmarks')} chevron />
          {!readOnly && (
            <Row icon={<Settings className="h-5 w-5" />} label="Nastavení vlákna" onClick={run(onOpenSettings)} />
          )}
          <li className="my-1 border-t border-zinc-200" role="separator" />
          {!readOnly && <Row icon={<Plus className="h-5 w-5" />} label="Nový chat" onClick={run(onNewChat)} />}
          <Row icon={<Search className="h-5 w-5" />} label="Hledat ve vlákně" onClick={run(onSearchInThread)} />
          {!readOnly && (
            <Row
              icon={<Trash2 className="h-5 w-5" />}
              label={confirmClear ? 'Opravdu smazat? Klepni znovu' : 'Smazat historii'}
              onClick={confirmClear ? run(onClearHistory) : () => setConfirmClear(true)}
              danger
            />
          )}
        </ul>
      </>
    )
  }

  return (
    <BottomSheet open={open} onClose={onClose} ariaLabel="Menu chatu" header={header}>
      {content}
    </BottomSheet>
  )
}
