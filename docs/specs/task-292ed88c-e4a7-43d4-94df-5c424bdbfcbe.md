Audit current Go test coverage and fill gaps. Focus on handlers, error paths, and business logic that isn't covered yet.

## Requirements

### Coverage audit
- Run `go test -coverprofile=coverage.out ./...` and `go tool cover -func=coverage.out` to see current coverage per package
- Identify packages and functions with low or zero coverage
- Prioritize: handlers > business logic > utilities

### Areas likely needing more tests
- **Handler edge cases**: invalid input, missing required fields, not-found IDs, duplicate entries
- **Runner package**: task scheduling logic, concurrent execution limits, retry with backoff
- **Claude package**: session pool lifecycle (eviction, expiry, model change), context assembly with all layers
- **MCP package**: edge cases in tool handling, error responses
- **Database**: migration rollback safety (up then down then up)
- **Projects**: git repo discovery edge cases (no .git, permission errors, symlinks)

### Test quality improvements
- Add table-driven tests where multiple inputs test the same function
- Add race condition tests for concurrent code (runner, session pool) — `go test -race` should pass
- Add benchmark tests for hot paths if any (context assembly, message parsing)
- Ensure all error paths are tested, not just happy paths

### Add coverage to CI
- Add `make test-coverage` target that runs tests with coverage and prints summary
- Optionally fail if coverage drops below a threshold (e.g. 60% — set a reasonable baseline)
- Add coverage report generation (`go tool cover -html=coverage.out -o coverage.html`)

## Implementation Notes
- Use existing test patterns — stdlib `testing`, `httptest`, Gin test mode
- No external test frameworks (no testify, no gomega — project uses stdlib only)
- Tests that need DB use `botka_test` database and auto-skip if unavailable
- Existing `make check` must still pass
- Run `go test -race ./...` to verify no race conditions

## Safety
**NEVER run `make deploy`, `make install-service`, `systemctl restart botka`, or any command that would restart the Botka service.**