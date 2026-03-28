The Botka app has a help page or section in the frontend. Add information explaining how Claude Code loads instruction files into conversation context:

1. **Global** — `~/.claude/CLAUDE.md` — applies to all projects, user's private instructions
2. **Project** — `<project>/CLAUDE.md` — applies only to that project, checked into the repo
3. **Auto-memory** — `~/.claude/projects/<encoded-dir>/memory/MEMORY.md` — automatic per-project memory

Find the help page in the frontend code (likely in frontend/src/pages/ or components/) and add a section that explains this hierarchy. Keep it concise and clear — written for a user who wants to understand how Claude Code reads instructions.

If no help page exists, find the closest suitable place in the frontend (settings, about, etc.) and add the info there.