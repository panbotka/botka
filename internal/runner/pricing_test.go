package runner

import (
	"math"
	"testing"
)

func TestLookupPricing_AliasAndCanonical(t *testing.T) {
	cases := []struct {
		model string
		want  string // expected family key in modelPrices
	}{
		{"sonnet", "sonnet"},
		{"Sonnet", "sonnet"},
		{"opus", "opus"},
		{"haiku", "haiku"},
		{"claude-sonnet-4-5-20250929", "sonnet"},
		{"claude-opus-4-7", "opus"},
		{"claude-haiku-4-5", "haiku"},
	}
	for _, c := range cases {
		t.Run(c.model, func(t *testing.T) {
			got, ok := lookupPricing(c.model)
			if !ok {
				t.Fatalf("lookupPricing(%q) not found", c.model)
			}
			want := modelPrices[c.want]
			if got != want {
				t.Errorf("lookupPricing(%q) = %+v, want %+v", c.model, got, want)
			}
		})
	}
}

func TestLookupPricing_Unknown(t *testing.T) {
	if _, ok := lookupPricing(""); ok {
		t.Error("empty model should not be found")
	}
	if _, ok := lookupPricing("gpt-5"); ok {
		t.Error("non-claude model should not be found")
	}
}

func TestComputeCost_KnownModel(t *testing.T) {
	// Sonnet: input $3/M, output $15/M, cache_read $0.30/M, cache_creation $3.75/M.
	// 1M input + 1M output + 1M cache_read + 1M cache_creation = 3 + 15 + 0.30 + 3.75 = 22.05
	got := computeCost("sonnet", 1_000_000, 1_000_000, 1_000_000, 1_000_000)
	want := 22.05
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("computeCost(sonnet) = %v, want %v", got, want)
	}
}

func TestComputeCost_UnknownModel(t *testing.T) {
	got := computeCost("gpt-5", 1000, 2000, 0, 0)
	if got != 0 {
		t.Errorf("computeCost(gpt-5) = %v, want 0 (unknown model)", got)
	}
}

func TestComputeCost_ZeroTokens(t *testing.T) {
	if got := computeCost("sonnet", 0, 0, 0, 0); got != 0 {
		t.Errorf("computeCost with zero tokens = %v, want 0", got)
	}
}
