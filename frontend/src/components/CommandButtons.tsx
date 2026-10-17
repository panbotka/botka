import { createPortal } from 'react-dom'
import { Play, Square, Rocket, RefreshCw } from 'lucide-react'
import type { Project } from '../types'
import { useProjectCommands } from '../hooks/useProjectCommands'

interface Props {
  project: Project | undefined
  /**
   * `compact` is the small pill row of the desktop chat header. `sheet` renders
   * the same actions as labelled, touch-sized (≥ 44px) buttons under a heading,
   * for the mobile chat "⋯" menu.
   */
  variant?: 'compact' | 'sheet'
}

export default function CommandButtons({ project, variant = 'compact' }: Props) {
  const {
    hasDevCommand,
    hasDeployCommand,
    runningDev,
    runningDeploy,
    toast,
    restarting,
    confirmDeploy,
    setConfirmDeploy,
    handleRun,
    handleKill,
    handleRestart,
    confirmAndDeploy,
  } = useProjectCommands(project)

  if (!hasDevCommand && !hasDeployCommand) return null

  const sheet = variant === 'sheet'
  // Pill sizing: compact header pills vs touch-sized sheet buttons.
  const pill = sheet ? 'min-h-[44px] px-3 text-sm gap-1.5' : 'px-2 py-1 text-xs gap-1'
  const icon = sheet ? 'h-4 w-4' : 'h-3 w-3'
  const iconBtn = sheet ? 'min-w-[44px] min-h-[44px] flex items-center justify-center' : 'p-0.5'
  // Labels hide on narrow screens in the compact header; the sheet always shows them.
  const label = sheet ? '' : 'hidden sm:inline'

  const buttons = (
    <div className={sheet ? 'flex flex-wrap items-center gap-2' : 'flex items-center gap-1.5 flex-shrink-0'}>
      {/* Dev button */}
      {hasDevCommand && (
        runningDev ? (
          <div className={`inline-flex items-center rounded-md bg-emerald-100 font-medium text-emerald-800 ${pill}`}>
            <span className="relative flex h-2 w-2 flex-shrink-0">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-500 opacity-75" />
              <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-600" />
            </span>
            {runningDev.port ? (
              <a
                href={`http://pi:${runningDev.port}`}
                target="_blank"
                rel="noopener noreferrer"
                className="hover:underline"
                title={`Open http://pi:${runningDev.port}`}
              >
                pi:{runningDev.port}
              </a>
            ) : (
              <span className={label}>Dev</span>
            )}
            <span className={`text-emerald-600 ${label}`} title={`PID ${runningDev.pid}`}>
              ({runningDev.pid})
            </span>
            <button
              onClick={handleRestart}
              disabled={restarting}
              title="Restart Dev"
              className={`ml-0.5 rounded ${iconBtn} hover:bg-emerald-200 transition-colors cursor-pointer disabled:opacity-50`}
            >
              <RefreshCw className={`${icon} ${restarting ? 'animate-spin' : ''}`} />
            </button>
            <button
              onClick={() => handleKill(runningDev.pid)}
              title="Stop Dev"
              className={`rounded ${iconBtn} hover:bg-emerald-200 transition-colors cursor-pointer`}
            >
              <Square className={icon} />
            </button>
          </div>
        ) : (
          <button
            onClick={() => handleRun('dev')}
            title="Start Dev"
            className={`inline-flex items-center rounded-md bg-zinc-100 font-medium text-zinc-600 ${pill} hover:bg-emerald-100 hover:text-emerald-700 transition-colors cursor-pointer`}
          >
            <Play className={icon} />
            <span className={label}>Dev</span>
          </button>
        )
      )}

      {/* Deploy button */}
      {hasDeployCommand && (
        runningDeploy ? (
          <div className={`inline-flex items-center rounded-md bg-blue-100 font-medium text-blue-800 ${pill}`}>
            <span className="relative flex h-2 w-2 flex-shrink-0">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-blue-500 opacity-75" />
              <span className="relative inline-flex rounded-full h-2 w-2 bg-blue-600" />
            </span>
            <span className={label}>Deploying...</span>
            <button
              onClick={() => handleKill(runningDeploy.pid)}
              title="Stop Deploy"
              className={`ml-0.5 rounded ${iconBtn} hover:bg-blue-200 transition-colors cursor-pointer`}
            >
              <Square className={icon} />
            </button>
          </div>
        ) : (
          <button
            onClick={() => setConfirmDeploy(true)}
            title="Deploy"
            className={`inline-flex items-center rounded-md bg-zinc-100 font-medium text-zinc-600 ${pill} hover:bg-blue-100 hover:text-blue-700 transition-colors cursor-pointer`}
          >
            <Rocket className={icon} />
            <span className={label}>Deploy</span>
          </button>
        )
      )}

      {/* Toast */}
      {toast && (
        <span className="rounded-md bg-zinc-800 px-2 py-1 text-xs text-white whitespace-nowrap">
          {toast}
        </span>
      )}
    </div>
  )

  return (
    <>
      {sheet ? (
        <div className="px-4 py-2">
          <div className="pb-1.5 text-[11px] font-medium uppercase tracking-wider text-zinc-400">Příkazy</div>
          {buttons}
        </div>
      ) : buttons}

      {/* Deploy confirmation dialog — portalled so a transformed ancestor
          (the mobile bottom sheet) cannot re-anchor its fixed overlay. */}
      {confirmDeploy && createPortal(
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setConfirmDeploy(false)}>
          <div className="mx-4 w-full max-w-md rounded-lg bg-white dark:bg-zinc-100 p-6 shadow-xl" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-lg font-semibold text-zinc-900">Confirm Deploy</h3>
            <p className="mt-2 text-sm text-zinc-600">
              This will execute the following command:
            </p>
            <pre className="mt-2 rounded-md bg-zinc-100 px-3 py-2 text-sm font-mono text-zinc-800">
              {project?.deploy_command}
            </pre>
            <div className="mt-4 flex justify-end gap-3">
              <button
                onClick={() => setConfirmDeploy(false)}
                className="rounded-md px-3 py-1.5 text-sm font-medium text-zinc-600 hover:bg-zinc-100 cursor-pointer"
              >
                Cancel
              </button>
              <button
                onClick={confirmAndDeploy}
                className="rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 cursor-pointer"
              >
                Deploy
              </button>
            </div>
          </div>
        </div>,
        document.body,
      )}
    </>
  )
}
