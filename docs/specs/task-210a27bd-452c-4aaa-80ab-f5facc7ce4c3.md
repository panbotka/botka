Audit all API endpoints for input validation and ensure consistent error response format across the entire API surface.

## Requirements

### Input validation audit
- Go through every handler in `internal/handlers/`
- For each POST/PUT endpoint, verify:
  - Required fields are checked (return 400 if missing)
  - String fields have reasonable length limits
  - Numeric fields have range checks where applicable
  - IDs are valid format (int64 or UUID)
  - Enum values are validated (status, role, etc.)
- Currently some handlers may just pass raw input to GORM and rely on DB constraints — add explicit validation before DB calls

### Consistent error responses
- All errors should follow the envelope: `{"error": "human readable message"}`
- Standardize HTTP status codes:
  - 400: bad request (validation errors, malformed input)
  - 401: unauthorized
  - 403: forbidden (valid auth but no permission)
  - 404: not found
  - 409: conflict (duplicate, already exists)
  - 500: internal server error (unexpected)
- Create a helper function if one doesn't exist: `respondError(c *gin.Context, status int, msg string)`
- Ensure no endpoint leaks internal error details (DB errors, stack traces) to the client

### Validation helper
- Create a simple validation helper in `internal/handlers/` or a new `internal/validation/` package:
  - `ValidateRequired(fields map[string]string) error` — checks non-empty
  - `ValidateMaxLength(field, value string, max int) error`
  - `ValidateEnum(field, value string, allowed []string) error`
- Keep it simple — no external validation library needed
- Or use struct tags with Gin's built-in binding validation (`binding:"required,max=200"`)

### Document API errors
- Add a comment block at the top of each handler listing possible error responses
- This helps future development and API consumers

## Implementation Notes
- Gin already supports `ShouldBindJSON` with struct tags for validation — leverage this
- Check if there's already a `respondError` helper and standardize on it
- Don't over-validate internal fields that users can't control
- Focus on user-facing input: message content, thread names, task specs, URLs, etc.
- Existing tests must pass (`make check`)
- Update existing handler tests to verify error responses

## Safety
**NEVER run `make deploy`, `make install-service`, `systemctl restart botka`, or any command that would restart the Botka service.**