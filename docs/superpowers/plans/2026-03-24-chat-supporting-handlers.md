# Chat Supporting Handlers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port all remaining Chatovadlo handlers (tags, personas, memories, search, transcribe, processes, status) to Botka's Gin+GORM stack.

**Architecture:** Each handler follows the established Botka pattern: struct with `*gorm.DB` + config fields, constructor `NewXHandler()`, route registration `RegisterXRoutes()`, response envelope via `respondOK`/`respondList`/`respondError`. All CRUD uses GORM directly. Search uses `db.Raw()` for PostgreSQL full-text search. Transcribe proxies to OpenClaw whisper endpoint.

**Tech Stack:** Go, Gin, GORM, PostgreSQL tsvector/ts_headline

---

### Task 1: Tag Handler

**Files:**
- Create: `internal/handlers/tag.go`

- [ ] **Step 1: Write tag handler** — CRUD for tags + thread count endpoint
- [ ] **Step 2: Verify compilation** — `go build ./...`

### Task 2: Persona Handler

**Files:**
- Create: `internal/handlers/persona.go`

- [ ] **Step 1: Write persona handler** — CRUD ordered by sort_order
- [ ] **Step 2: Verify compilation** — `go build ./...`

### Task 3: Memory Handler

**Files:**
- Create: `internal/handlers/memory.go`

- [ ] **Step 1: Write memory handler** — CRUD with UUID primary keys
- [ ] **Step 2: Verify compilation** — `go build ./...`

### Task 4: Search Handler

**Files:**
- Create: `internal/handlers/search.go`

- [ ] **Step 1: Write search handler** — Full-text search with tsvector, ts_headline, grouped by thread
- [ ] **Step 2: Verify compilation** — `go build ./...`

### Task 5: Transcribe Handler

**Files:**
- Create: `internal/handlers/transcribe.go`

- [ ] **Step 1: Write transcribe handler** — Status check + whisper proxy via multipart forwarding
- [ ] **Step 2: Verify compilation** — `go build ./...`

### Task 6: Process Handler

**Files:**
- Create: `internal/handlers/process.go`

- [ ] **Step 1: Write process handler** — List/kill via claude.Registry
- [ ] **Step 2: Verify compilation** — `go build ./...`

### Task 7: Status Handler

**Files:**
- Create: `internal/handlers/status.go`

- [ ] **Step 1: Write status handler** — Models list + server status
- [ ] **Step 2: Verify compilation** — `go build ./...`

### Task 8: Wire Routes

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Register all new handlers in setupRouter**
- [ ] **Step 2: Full build + test** — `go build ./...` and `go test ./...`
- [ ] **Step 3: Commit**
