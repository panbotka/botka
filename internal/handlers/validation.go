package handlers

import (
	"fmt"
	"strings"
)

// maxTitleLength is the maximum length for title fields (tasks, threads, personas, tags).
const maxTitleLength = 500

// maxSpecLength is the maximum length for task spec fields.
const maxSpecLength = 100000

// maxContentLength is the maximum length for message/memory content fields.
const maxContentLength = 100000

// maxURLLength is the maximum length for URL fields.
const maxURLLength = 2048

// maxLabelLength is the maximum length for label fields.
const maxLabelLength = 200

// maxUsernameLength is the maximum length for username fields.
const maxUsernameLength = 100

// maxPasswordLength is the maximum length for password fields (bcrypt has a 72-byte limit).
const maxPasswordLength = 72

// maxSystemPromptLength is the maximum length for persona system prompts.
const maxSystemPromptLength = 100000

// maxClaudeMDLength is the maximum length for project claude_md fields.
const maxClaudeMDLength = 200000

// maxVerificationCmdLength is the maximum length for project verification commands.
const maxVerificationCmdLength = 1000

// validateRequired checks that a string field is non-empty after trimming whitespace.
// Returns an error message like "<field> is required" if empty.
func validateRequired(field, value string) string {
	if strings.TrimSpace(value) == "" {
		return field + " is required"
	}
	return ""
}

// validateMaxLength checks that a string field does not exceed the given maximum length.
// Returns an error message like "<field> must be at most N characters" if too long.
func validateMaxLength(field, value string, maxLen int) string {
	if len(value) > maxLen {
		return fmt.Sprintf("%s must be at most %d characters", field, maxLen)
	}
	return ""
}

// validateEnum checks that a string value is one of the allowed values.
// Returns an error message like "<field> must be one of: a, b, c" if invalid.
func validateEnum(field, value string, allowed []string) string {
	for _, a := range allowed {
		if value == a {
			return ""
		}
	}
	return fmt.Sprintf("%s must be one of: %s", field, strings.Join(allowed, ", "))
}

// firstError returns the first non-empty string from the given checks.
// Usage:
//
//	if msg := firstError(
//	    validateRequired("title", req.Title),
//	    validateMaxLength("title", req.Title, 500),
//	); msg != "" {
//	    respondError(c, 400, msg)
//	}
func firstError(checks ...string) string {
	for _, msg := range checks {
		if msg != "" {
			return msg
		}
	}
	return ""
}
