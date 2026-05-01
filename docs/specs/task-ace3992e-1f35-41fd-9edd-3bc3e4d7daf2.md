# Track token usage and cost per task

Persist Claude API token consumption for every task execution and surface aggregates in the UI.

## Requirements

- Add columns to the `tasks` table for `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_creation_tokens`, and `cost_usd` (nullable, populated after execution).
- Update the runner/executor to parse the final `result` event from the Claude Code stdout stream and record the token counts and computed cost.
- Cost is computed using model-specific pricing — keep a small pricing table keyed by model name, fall back to 0 with a warning log if the model is unknown.
- Expose the totals in the task detail API response and render them on the task detail page (formatted tokens + USD with 4 decimals).
- Add a per-project aggregate endpoint returning total tokens, total cost, and average cost per task; surface it on the project page.
- Existing tasks without data show "—" instead of zeros.

## Implementation Notes

- Token data is already in the `result` NDJSON event Claude Code emits — no extra API calls needed.
- Add a migration pair under `migrations/`.
