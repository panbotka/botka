# Web Push Backend

Add server-side Web Push infrastructure (VAPID, subscriptions, send helper) so Botka can deliver push notifications to subscribed browsers when the app tab is closed or backgrounded.

## Context

Botka currently uses the in-tab `Notification` API in `frontend/src/hooks/useNotifications.ts`, which only fires when the tab is open. This task lays the foundation for true Web Push that works even when the browser is closed (PWA installed) or the tab is minimised.

This task is **backend only**: VAPID key handling, subscription model, REST endpoints, and a Send helper. Event triggers (task state, chat reply) and the frontend service worker are separate, dependent pieces of work.

## Requirements

### Migration

- Create `migrations/027_push_subscriptions.up.sql` and `.down.sql`.
- Up:
  ```
  CREATE TABLE push_subscriptions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    UNIQUE (user_id, endpoint)
  );
  CREATE INDEX idx_push_subscriptions_user_id ON push_subscriptions (user_id);
  ```
- Down: drop the table.

### Model

- Add `internal/models/push_subscription.go` with `PushSubscription` struct mapping all columns. `TableName()` returns `"push_subscriptions"`.
- Use `int64` PK (consistent with other chat-side models like `Message`, `Thread`).

### Config

- Add three env vars to `internal/config/config.go`, all optional strings:
  - `VAPID_PUBLIC_KEY` (base64url)
  - `VAPID_PRIVATE_KEY` (base64url)
  - `VAPID_SUBJECT` (default `mailto:kozak@talko.cz`)
- If either key is empty, push is disabled — endpoints return 503 and the Sender is nil. Document in `CLAUDE.md` Environment Variables table.

### CLI helper

- Add a `botka vapid-generate` subcommand in `cmd/server/main.go` that generates a VAPID key pair and prints to stdout in `KEY=value` form ready to paste into `.env`:
  ```
  VAPID_PUBLIC_KEY=...
  VAPID_PRIVATE_KEY=...
  ```
- Use `github.com/SherClockHolmes/webpush-go` for both key generation and push sending. Add it to `go.mod`.

### Push package

- Create `internal/push/push.go` with:
  - `type PushPayload struct { Title, Body, URL, Tag string }`.
  - `type Sender interface { Send(ctx context.Context, userID int64, payload PushPayload) error; Broadcast(ctx context.Context, payload PushPayload) error }` — using an interface keeps trigger code testable with a fake.
  - `type webPushSender struct { ... }` implementing the interface, with VAPID config and `*gorm.DB`.
  - `func NewSender(cfg config.Config, db *gorm.DB) (Sender, error)` — returns nil + error if VAPID keys missing or invalid; callers should treat nil Sender as "push disabled".
  - `Send` fetches all subscriptions for `userID` and sends the same JSON payload to each. Bound concurrent sends with a worker pool (max 10).
  - `Broadcast` fetches all subscriptions and sends to every one (used for system-wide notifications).
  - On 404 or 410 response from the push service, delete the dead subscription row and continue.
  - Update `last_used_at` on successful send.
  - Errors per-subscription are logged but not surfaced to the caller; `Send` returns nil unless a programming error occurred (e.g. payload marshal failure).

### Endpoints

Mount under `/api/v1/push/`. All require an authenticated user (use the same auth middleware as other authenticated endpoints).

- `GET /api/v1/push/vapid-public-key` → `{"data": {"public_key": "<base64url>"}}`. Returns 503 with `{"error": "push notifications not configured"}` when VAPID disabled.
- `POST /api/v1/push/subscriptions` body `{"endpoint": "...", "keys": {"p256dh": "...", "auth": "..."}, "user_agent": "..."}` → 201 `{"data": <subscription>}`. Upsert on `(user_id, endpoint)` (returns existing row if duplicate).
- `GET /api/v1/push/subscriptions` → `{"data": [<subscriptions for current user>], "total": N}`.
- `DELETE /api/v1/push/subscriptions/:id` → 204. Only deletes when the row's `user_id` matches the current user; returns 404 otherwise.
- `POST /api/v1/push/test` body `{"title": "...", "body": "..."}` (both optional) → triggers `Sender.Send` to current user with a default test payload (`title: "Test from Botka"`, `body: "Push notifications are working"`, `url: "/"`). Returns 200 `{"data": {"sent": <count>}}`.

### Wiring

- Construct the Sender at startup in `cmd/server/main.go`. If VAPID keys are missing, log a warning and skip; do not crash.
- Inject Sender into the push handler. Do not stash in a global; pass via handler struct.

### Tests

- Unit tests for `internal/push/push.go` using a stub HTTP server (no real push service): success, dead subscription cleanup on 410, payload marshalling.
- Handler integration tests for all five endpoints (skip if `botka_test` DB unavailable, per project convention): unauthenticated (401), VAPID disabled (503), create subscription (idempotency), list, delete (own vs other user), test endpoint.

## Edge Cases

- Two browsers from the same user: each registers its own subscription row. Both receive every push.
- Subscription endpoint expires (push service returns 410 Gone): row is deleted; sends to other subscriptions continue.
- VAPID keys missing: all `/api/v1/push/*` endpoints return 503; `NewSender` returns nil so trigger code paths can short-circuit.
- Subscription with malformed keys (cannot decode): `POST /api/v1/push/subscriptions` returns 400.

## Out of Scope

- Triggering pushes from task or chat events. Only the infrastructure and the test endpoint here.
- Frontend service worker and subscription UI.
