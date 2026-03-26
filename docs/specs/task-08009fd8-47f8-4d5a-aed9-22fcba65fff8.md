# Show session health — token count and context limit proximity

Display how many tokens the current session has used and how close it is to the context window limit. Also fix the existing hardcoded 200k context bar in ChatView.tsx.

## Bug fix: existing context bar
- `ChatView.tsx` line 919 hardcodes `contextWindow = 200000` for all models
- It shows tokens from only the last message, not cumulative session usage
- Correct context limits per model:
  - **opus**: 1,000,000 tokens
  - **sonnet**: 200,000 tokens
  - **haiku**: 200,000 tokens
- The bar should use the correct limit based on the thread's model setting
- The `input_tokens` from the last result event already approximates current context usage (it includes the full conversation sent to the API), so using it is correct — but the limit must match the model

## Requirements

### Backend — Track cumulative session tokens
- Track cumulative `input_tokens` and `output_tokens` per active session in the SessionManager
- After each message completes (result event), add the token counts to a running total on the Session struct
- Add `GET /api/v1/threads/:id/session-health` endpoint returning:
  - `{"data": {"active": true, "total_input_tokens": N, "total_output_tokens": N, "estimated_context_tokens": N, "context_limit": N, "context_usage_pct": N, "model": "...", "started_at": "...", "message_count": N}}`
- `estimated_context_tokens`: input_tokens from the last message (represents what Claude "sees")
- `context_limit`: model-dependent limit (1M for opus, 200k for sonnet/haiku)
- `context_usage_pct`: percentage of context used (0-100)
- If no active session, return `{"data": {"active": false}}`

### Frontend — Fix context bar
- Replace the hardcoded `200000` in ChatView.tsx with the correct limit based on thread model
- Model is available on the thread object — map it to the correct limit
- Keep using `input_tokens` from the usage event for the bar value (this is correct)

### Frontend — Session health indicator
- Show a small, unobtrusive indicator in the chat thread header area
- Display: context usage as a progress bar or percentage (e.g. "42% context")
- Color coding: green (<50%), yellow (50-80%), red (>80%)
- Tooltip or expandable detail: total input/output tokens, message count, session uptime
- Update after each message completes (from the usage SSE event data)

### Frontend — Styling
- Small and subtle — should not dominate the chat UI
- Follow existing design language
- Only visible when there's an active session

## Implementation Notes

- Changes needed in `ChatView.tsx` around line 919 for the immediate fix
- Session struct in `pool.go` already exists — add token tracking fields
- The usage SSE event already sends `input_tokens` and `output_tokens` to the frontend
- Model context limits should be defined in one place (backend config or shared constant) — avoid hardcoding in multiple files
- Existing tests must pass (`make check`)

## Safety

**NEVER run `make deploy`, `make install-service`, `systemctl restart botka`, or any command that would restart the Botka service.**