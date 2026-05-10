# Task notes/comments

Allow attaching free-form, timestamped notes to a task — useful for triage, follow-ups, and context that doesn't belong in the spec.

## Background

The `spec` is the input handed to Claude. Once a task is running or done, there's no place to capture human observations: "this failure looks like a flaky test, retry tomorrow", "depends on PR #42 landing first", "see Slack thread X for context". Today this lives nowhere.

## Requirements

- New table `task_notes`: `id` (bigserial), `task_id` (uuid, FK, ON DELETE CASCADE), `body` (text, required, non-empty after trim), `author` (text, defaults to "user"), `created_at`, `updated_at`.
- Notes are append-only from a UX standpoint, but support edit + delete (soft delete with `deleted_at`).
- HTTP API:
  - `GET /api/v1/tasks/:id/notes` — list notes for a task, ordered by `created_at` ASC.
  - `POST /api/v1/tasks/:id/notes` — create note (`body`).
  - `PATCH /api/v1/tasks/:id/notes/:note_id` — edit body (sets `updated_at`).
  - `DELETE /api/v1/tasks/:id/notes/:note_id` — soft-delete.
- Notes are NOT included in the context handed to Claude when the task runs — they are human-only metadata.
- Task list response gains `notes_count` (excluding soft-deleted) so the UI can show a badge.
- Frontend:
  - In task detail, add a "Notes" section below the spec.
  - Markdown rendering for note bodies.
  - Inline edit + delete with confirmation.
  - "Add note" form with Cmd+Enter to submit.
  - Show notes_count badge in task list rows.
- MCP: add `add_task_note` and `list_task_notes` tools.

## Implementation Notes

- Soft-delete pattern: use `gorm.DeletedAt` like `messages` and `attachments`. Raw SQL queries (if any) must filter `deleted_at IS NULL`.
- Keep this strictly out of the runner's context-assembly path — there is no scenario where notes should be sent to Claude.
- `author` field is forward-looking: today it's always "user", but reserves room for "agent"/"system" later (e.g. an LLM commenting on its own failure).
