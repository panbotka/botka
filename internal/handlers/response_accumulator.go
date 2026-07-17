package handlers

import "strings"

// responseAccumulator builds the assistant's final text from one message's
// event stream, which may span multiple turns. Newer Claude Code can end a turn
// early after launching a background subagent and then run a second turn once it
// finishes, so a single reply can carry several result events.
//
// Content deltas append directly. A turn's result text is appended only when
// that turn produced no streamed deltas — otherwise the result text merely
// repeats what already streamed. This keeps every turn's text (separated by a
// blank line) instead of dropping all but the first.
type responseAccumulator struct {
	buf          strings.Builder
	turnStreamed bool
}

// addDelta appends a streamed text chunk for the current turn.
func (a *responseAccumulator) addDelta(text string) {
	a.buf.WriteString(text)
	a.turnStreamed = true
}

// addResult records a turn's final result text and ends the turn. The text is
// appended only if this turn streamed nothing (so it doesn't duplicate deltas),
// separated from any earlier turn's text by a blank line.
func (a *responseAccumulator) addResult(resultText string) {
	if resultText != "" && !a.turnStreamed {
		if a.buf.Len() > 0 {
			a.buf.WriteString("\n\n")
		}
		a.buf.WriteString(resultText)
	}
	a.turnStreamed = false
}

// String returns the accumulated response text.
func (a *responseAccumulator) String() string { return a.buf.String() }

// Len reports the accumulated length in bytes.
func (a *responseAccumulator) Len() int { return a.buf.Len() }
