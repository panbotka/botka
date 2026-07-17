package handlers

import "testing"

func TestResponseAccumulator_SingleNonStreamedTurn(t *testing.T) {
	var a responseAccumulator
	a.addResult("hello")
	if got := a.String(); got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestResponseAccumulator_StreamedTurnDoesNotDuplicateResult(t *testing.T) {
	// When a turn streams its text as deltas, its result text is the same text
	// again and must not be appended twice.
	var a responseAccumulator
	a.addDelta("hel")
	a.addDelta("lo")
	a.addResult("hello")
	if got := a.String(); got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestResponseAccumulator_MultiTurnKeepsEveryTurn(t *testing.T) {
	// The item-2 case: the model ends a turn early ("launched"), then a second
	// turn reports the background result. Both non-streamed turns must survive.
	var a responseAccumulator
	a.addResult("launched")
	a.addResult("Background task completed.")
	want := "launched\n\nBackground task completed."
	if got := a.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResponseAccumulator_MultiTurnStreamedNotDuplicated(t *testing.T) {
	// Two streamed turns: deltas carry the text, result texts must be dropped.
	var a responseAccumulator
	a.addDelta("a")
	a.addResult("a")
	a.addDelta("b")
	a.addResult("b")
	if got := a.String(); got != "ab" {
		t.Fatalf("got %q, want %q", got, "ab")
	}
}

func TestResponseAccumulator_EmptyResultIgnored(t *testing.T) {
	var a responseAccumulator
	a.addResult("")
	if got := a.String(); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
