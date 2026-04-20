# Raise chat upload size limit to 500 MB

The chat attachment size limit is hardcoded at 10 MB in both backend and frontend. Raise it to 500 MB.

## Requirements

- `internal/handlers/chat.go:50` — change `const maxUploadSize = 10 << 20` to 500 MB. Both call sites at lines ~1000 and ~1063 continue to use the constant.
- `frontend/src/components/ChatInput.tsx:15` — change `MAX_FILE_SIZE = 10 * 1024 * 1024` to 500 MB. All consumers (`ChatInput.tsx:24`, `ChatView.tsx:197`) use the constant unchanged.
- Error messages shown to the user that mention the limit must reflect the new value (grep for "10 MB" / "10MB" across frontend and backend).
- Verify Gin's `MaxMultipartMemory` (default 32 MiB) is raised in the server bootstrap (`cmd/server/main.go` or wherever the `*gin.Engine` is created) to at least 512 MiB so large uploads do not fall back to temp files unnecessarily — or leave it at default and document that large uploads spill to disk. Pick the simpler option and note the choice in the commit message.
- No new environment variable — a constant change is sufficient. Keep both backend and frontend constants in sync.
- `make check` must pass. Add or update a test that asserts the upload handler rejects a file over 500 MB and accepts a file just under it (use a small value via an overridable var if the test would be too slow, or test the boundary check logic directly).

## Implementation Notes

- The upload path saves to `./data/uploads` on disk. No in-memory cap beyond Gin's multipart buffer — the OS tmp dir will hold the spill, so ensure `/tmp` has enough space on the target host (advisory, not blocking).
- Do not introduce a configurable env var unless the single constant change turns out to be insufficient for a specific concern.