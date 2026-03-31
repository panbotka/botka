## Goal

In the project detail view, show how many commits the local branch is ahead of the remote (i.e., unpushed commits). Something like "3 commits ahead of origin/main" or a simple "↑ 3" indicator.

## Backend

The project detail endpoint or git-status endpoint (`GET /api/v1/projects/:id/git-status`) should include an `ahead` count. Use `git rev-list --count origin/<branch>..HEAD` (or `@{upstream}..HEAD`) to get the number of unpushed commits.

Check `internal/handlers/project.go` — the `GetGitStatus` handler already runs git commands. Add the ahead count there.

Handle edge cases:
- No remote tracking branch → show "no remote" or omit
- Remote not fetchable → just use local ref comparison

## Frontend

Find the project detail component (likely in `frontend/src/pages/ProjectsPage.tsx` or a project detail component) and display the ahead count. If ahead > 0, show it prominently (e.g., yellow/orange badge "↑ 3 unpushed"). If 0, show a green checkmark or "up to date".

## Testing

- `make check` must pass
