# Chat Command Buttons UX Improvement

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve the CommandButtons component to show clickable address links, PID info, and a restart button for running dev servers.

**Architecture:** Two-file change — add `handleRestart` + `restarting` state to the `useProjectCommands` hook, then update `CommandButtons` to show the new running state layout with address link, restart button, and stop button.

**Tech Stack:** React 19, TypeScript, Tailwind CSS 4, Lucide icons

---

## File Map

- **Modify:** `frontend/src/hooks/useProjectCommands.ts` — add `handleRestart` function and `restarting` state
- **Modify:** `frontend/src/components/CommandButtons.tsx` — new running state layout with address link, restart/stop buttons, PID display

---

### Task 1: Add restart logic to useProjectCommands hook

**Files:**
- Modify: `frontend/src/hooks/useProjectCommands.ts`

- [ ] **Step 1: Add `restarting` state and `handleRestart` to the interface**

In `frontend/src/hooks/useProjectCommands.ts`, update the `UseProjectCommandsResult` interface to add the new exports:

```typescript
interface UseProjectCommandsResult {
  hasDevCommand: boolean
  hasDeployCommand: boolean
  runningDev: RunningCommandStatus | undefined
  runningDeploy: RunningCommandStatus | undefined
  toast: string | null
  restarting: boolean
  confirmDeploy: boolean
  setConfirmDeploy: (v: boolean) => void
  handleRun: (type: 'dev' | 'deploy') => void
  handleKill: (pid: number) => void
  handleRestart: () => void
  confirmAndDeploy: () => void
}
```

- [ ] **Step 2: Implement `handleRestart` and `restarting` state**

Add `restarting` state at the top of the hook, next to the other state declarations:

```typescript
const [restarting, setRestarting] = useState(false)
```

Add the `handleRestart` function after `handleKill`:

```typescript
const handleRestart = useCallback(async () => {
  if (!projectId) return
  const devCmd = commands.find((c) => c.command_type === 'dev')
  if (!devCmd) return
  setRestarting(true)
  showToast('Restarting dev...')
  try {
    await killProjectCommand(projectId, devCmd.pid)
    // Wait for port to free up
    await new Promise((r) => setTimeout(r, 1500))
    const result = await runProjectCommand(projectId, 'dev')
    showToast(`Dev restarted (PID ${result.pid})`)
    loadCommands()
  } catch (err) {
    showToast(err instanceof Error ? err.message : 'Failed to restart', 4000)
  } finally {
    setRestarting(false)
  }
}, [projectId, commands, showToast, loadCommands])
```

- [ ] **Step 3: Add `restarting` and `handleRestart` to the return object**

Update the return statement to include the new exports:

```typescript
return {
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
}
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd /home/pi/projects/botka/frontend && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add frontend/src/hooks/useProjectCommands.ts
git commit -m "feat: add handleRestart and restarting state to useProjectCommands hook"
```

---

### Task 2: Update CommandButtons running dev layout

**Files:**
- Modify: `frontend/src/components/CommandButtons.tsx`

- [ ] **Step 1: Add RefreshCw import and destructure new hook values**

Update the import line and the hook destructuring:

```typescript
import { Play, Square, Rocket, RefreshCw } from 'lucide-react'
```

Add `handleRestart` and `restarting` to the destructured values from the hook:

```typescript
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
```

- [ ] **Step 2: Replace the running dev button with the new layout**

Replace the entire `runningDev ? (...)` block (the single stop button) with the new layout showing address link + restart + stop:

```tsx
runningDev ? (
  <div className="inline-flex items-center gap-1 rounded-md bg-emerald-100 px-2 py-1 text-xs font-medium text-emerald-800">
    <span className="relative flex h-2 w-2 flex-shrink-0">
      <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-500 opacity-75" />
      <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-600" />
    </span>
    {runningDev.port ? (
      <a
        href={`http://${window.location.hostname}:${runningDev.port}`}
        target="_blank"
        rel="noopener noreferrer"
        className="hover:underline"
        title={`Open http://${window.location.hostname}:${runningDev.port}`}
      >
        {window.location.hostname}:{runningDev.port}
      </a>
    ) : (
      <span className="hidden sm:inline">Dev</span>
    )}
    <span className="text-emerald-600 hidden sm:inline" title={`PID ${runningDev.pid}`}>
      ({runningDev.pid})
    </span>
    <button
      onClick={handleRestart}
      disabled={restarting}
      title="Restart Dev"
      className="ml-0.5 rounded p-0.5 hover:bg-emerald-200 transition-colors cursor-pointer disabled:opacity-50"
    >
      <RefreshCw className={`h-3 w-3 ${restarting ? 'animate-spin' : ''}`} />
    </button>
    <button
      onClick={() => handleKill(runningDev.pid)}
      title="Stop Dev"
      className="rounded p-0.5 hover:bg-emerald-200 transition-colors cursor-pointer"
    >
      <Square className="h-3 w-3" />
    </button>
  </div>
)
```

- [ ] **Step 3: Update the running deploy layout**

Replace the running deploy block with the improved layout showing "Deploying..." and stop button:

```tsx
runningDeploy ? (
  <div className="inline-flex items-center gap-1 rounded-md bg-blue-100 px-2 py-1 text-xs font-medium text-blue-800">
    <span className="relative flex h-2 w-2 flex-shrink-0">
      <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-blue-500 opacity-75" />
      <span className="relative inline-flex rounded-full h-2 w-2 bg-blue-600" />
    </span>
    <span className="hidden sm:inline">Deploying...</span>
    <button
      onClick={() => handleKill(runningDeploy.pid)}
      title="Stop Deploy"
      className="ml-0.5 rounded p-0.5 hover:bg-blue-200 transition-colors cursor-pointer"
    >
      <Square className="h-3 w-3" />
    </button>
  </div>
)
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd /home/pi/projects/botka/frontend && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 5: Verify full check passes**

Run: `cd /home/pi/projects/botka && make check`
Expected: All checks pass (fmt, vet, lint, test, frontend type-check)

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/CommandButtons.tsx
git commit -m "feat: add restart button, clickable address link, and PID display to command buttons"
```
