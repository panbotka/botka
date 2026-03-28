import { Play, Square, Rocket } from 'lucide-react'
import type { Project } from '../types'
import { useProjectCommands } from '../hooks/useProjectCommands'

export default function CommandButtons({ project }: { project: Project | undefined }) {
  const {
    hasDevCommand,
    hasDeployCommand,
    runningDev,
    runningDeploy,
    toast,
    confirmDeploy,
    setConfirmDeploy,
    handleRun,
    handleKill,
    confirmAndDeploy,
  } = useProjectCommands(project)

  if (!hasDevCommand && !hasDeployCommand) return null

  return (
    <>
      <div className="flex items-center gap-1.5 flex-shrink-0">
        {/* Dev button */}
        {hasDevCommand && (
          runningDev ? (
            <button
              onClick={() => handleKill(runningDev.pid)}
              title={`Stop Dev${runningDev.port ? ` on :${runningDev.port}` : ''} (PID ${runningDev.pid})`}
              className="inline-flex items-center gap-1 rounded-md bg-emerald-100 px-2 py-1 text-xs font-medium text-emerald-800 hover:bg-emerald-200 transition-colors cursor-pointer"
            >
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-500 opacity-75" />
                <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-600" />
              </span>
              <Square className="h-3 w-3" />
              <span className="hidden sm:inline">Dev</span>
            </button>
          ) : (
            <button
              onClick={() => handleRun('dev')}
              title="Start Dev"
              className="inline-flex items-center gap-1 rounded-md bg-zinc-100 px-2 py-1 text-xs font-medium text-zinc-600 hover:bg-emerald-100 hover:text-emerald-700 transition-colors cursor-pointer"
            >
              <Play className="h-3 w-3" />
              <span className="hidden sm:inline">Dev</span>
            </button>
          )
        )}

        {/* Deploy button */}
        {hasDeployCommand && (
          runningDeploy ? (
            <button
              onClick={() => handleKill(runningDeploy.pid)}
              title={`Stop Deploy${runningDeploy.port ? ` on :${runningDeploy.port}` : ''} (PID ${runningDeploy.pid})`}
              className="inline-flex items-center gap-1 rounded-md bg-blue-100 px-2 py-1 text-xs font-medium text-blue-800 hover:bg-blue-200 transition-colors cursor-pointer"
            >
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-blue-500 opacity-75" />
                <span className="relative inline-flex rounded-full h-2 w-2 bg-blue-600" />
              </span>
              <Square className="h-3 w-3" />
              <span className="hidden sm:inline">Deploy</span>
            </button>
          ) : (
            <button
              onClick={() => setConfirmDeploy(true)}
              title="Deploy"
              className="inline-flex items-center gap-1 rounded-md bg-zinc-100 px-2 py-1 text-xs font-medium text-zinc-600 hover:bg-blue-100 hover:text-blue-700 transition-colors cursor-pointer"
            >
              <Rocket className="h-3 w-3" />
              <span className="hidden sm:inline">Deploy</span>
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

      {/* Deploy confirmation dialog */}
      {confirmDeploy && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setConfirmDeploy(false)}>
          <div className="mx-4 w-full max-w-md rounded-lg bg-white p-6 shadow-xl" onClick={(e) => e.stopPropagation()}>
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
        </div>
      )}
    </>
  )
}
