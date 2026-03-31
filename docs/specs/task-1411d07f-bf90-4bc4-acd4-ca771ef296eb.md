## Goal

Add a section to the help page (`/help`) explaining that the task scheduler pauses when API rate limits are approached, and explicitly state the current threshold values.

## Content to Add

Add a section (e.g. "Rate Limits" or "Scheduler & Rate Limits") that explains:

- The task scheduler automatically pauses when Anthropic API usage approaches rate limits
- **5-hour window**: scheduler stops picking new tasks when utilization exceeds **90%** (`USAGE_THRESHOLD_5H = 0.90`)
- **7-day window**: scheduler stops picking new tasks when utilization exceeds **95%** (`USAGE_THRESHOLD_7D = 0.95`)
- Usage is checked every 30 seconds via the `claude-usage` command
- When the rate limit window resets or utilization drops below the threshold, the scheduler automatically resumes
- Chat sessions are NOT affected — only autonomous task execution is paused

## Implementation

Find the help page component in the frontend (search for `/help` route or "help" page component) and add the section. Use the same styling as existing help sections.

Read the current threshold values from the environment variables in `internal/config/` to confirm the defaults are correct.

## Testing

- `make check` must pass
