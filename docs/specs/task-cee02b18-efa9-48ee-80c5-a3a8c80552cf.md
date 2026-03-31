## Goal

Include `~/.openclaw/workspace/TOOLS.md` in the assembled chat context, right after USER.md (layer 2).

## Current State

`internal/claude/context.go` in `AssembleContext()` loads these files from `cfg.OpenClawWorkspace`:
- Layer 1: `SOUL.md` (identity)
- Layer 2: `USER.md` (user info)
- Layer 3: `MEMORY.md` (operational memory)

`TOOLS.md` is not loaded at all.

## Fix

Add TOOLS.md as a new layer between USER.md and MEMORY.md in `AssembleContext()` in `internal/claude/context.go`:

```go
// Layer 2: USER.md (user info)
if content, err := readFileIfExists(filepath.Join(cfg.OpenClawWorkspace, "USER.md")); err == nil && content != "" {
    parts = append(parts, "# About the User\n\n"+content)
}

// Layer 3: TOOLS.md (available tools and commands)
if content, err := readFileIfExists(filepath.Join(cfg.OpenClawWorkspace, "TOOLS.md")); err == nil && content != "" {
    parts = append(parts, "# Available Tools\n\n"+content)
}

// Layer 4: MEMORY.md (operational memory)  ← renumber
```

Update the function's doc comment to include TOOLS.md in the layer list and renumber subsequent layers.

## Help Page

Update the help/about page that is served at `/help`. Find the component that renders the context assembly documentation (search for "SOUL.md", "context layers", or "hierarchical context" in the frontend) and add TOOLS.md to the listed layers in the correct position (after USER.md, before MEMORY.md).

## Testing

- `make check` must pass
- Update or add a test in `context_test.go` that verifies TOOLS.md content appears in the assembled output when the file exists
