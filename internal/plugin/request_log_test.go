package plugin

import (
	"math"
	"testing"
	"time"
)

func TestRequestDetailForUsageSeparateReasoningTPS(t *testing.T) {
	usage := normalizedUsage{
		Dimensions:          Dimensions{Provider: "gemini", Model: "gemini-thinking"},
		RequestedAt:         time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
		LatencyNS:           uint64(2250 * time.Millisecond),
		TTFTNS:              uint64(250 * time.Millisecond),
		explicitTotalTokens: true,
		Counters: Counters{
			Requests:        1,
			InputTokens:     100,
			OutputTokens:    50,
			ReasoningTokens: 600,
			TotalTokens:     750,
		},
	}
	item := requestDetailForUsage(usage, 1)
	if item.GenerationNS != uint64(2*time.Second) {
		t.Fatalf("generation time = %d", item.GenerationNS)
	}
	if item.TPS != 325 {
		t.Fatalf("TPS = %v, want 325", item.TPS)
	}
}

func TestRequestDetailForUsageBufferedSeparateReasoningTPS(t *testing.T) {
	usage := normalizedUsage{
		Dimensions:  Dimensions{Provider: "gemini", Model: "gemini-thinking"},
		RequestedAt: time.Date(2026, 9, 17, 5, 10, 12, 0, time.UTC),
		LatencyNS:   uint64(8600 * time.Millisecond),
		TTFTNS:      uint64(7976 * time.Millisecond),
		Counters: Counters{
			Requests:        1,
			InputTokens:     200000,
			OutputTokens:    966,
			ReasoningTokens: 721,
			TotalTokens:     202687,
		},
	}
	item := requestDetailForUsage(usage, 1)
	if item.GenerationNS != uint64(624*time.Millisecond) {
		t.Fatalf("generation time = %d, want %d", item.GenerationNS, 624*time.Millisecond)
	}
	wantTPS := 1687.0 / 8.6
	if math.Abs(item.TPS-wantTPS) > 1e-9 {
		t.Fatalf("TPS = %v, want %v", item.TPS, wantTPS)
	}
	if item.TPSBasis != "latency_buffered" {
		t.Fatalf("TPS basis = %q, want latency_buffered", item.TPSBasis)
	}
}

func TestRequestDetailForUsageDoesNotFlagSmallNormalStream(t *testing.T) {
	usage := normalizedUsage{
		Dimensions: Dimensions{Provider: "gemini", Model: "gemini-thinking"},
		LatencyNS:  uint64(3 * time.Second),
		TTFTNS:     uint64(2 * time.Second),
		Counters: Counters{
			OutputTokens:    40,
			ReasoningTokens: 76,
		},
	}
	item := requestDetailForUsage(usage, 1)
	if item.TPS != 116 {
		t.Fatalf("TPS = %v, want 116", item.TPS)
	}
	if item.TPSBasis != "generation" {
		t.Fatalf("TPS basis = %q, want generation", item.TPSBasis)
	}
}

func TestRequestDetailForUsageDoesNotInventTPSFromShortBufferedLatency(t *testing.T) {
	usage := normalizedUsage{
		Dimensions: Dimensions{Provider: "gemini", Model: "gemini-thinking"},
		LatencyNS:  uint64(10 * time.Millisecond),
		TTFTNS:     uint64(9 * time.Millisecond),
		Counters: Counters{
			OutputTokens:    400,
			ReasoningTokens: 100,
		},
	}
	item := requestDetailForUsage(usage, 1)
	if item.TPS != 0 {
		t.Fatalf("TPS = %v, want 0 for unreliable timing", item.TPS)
	}
	if item.TPSBasis != "latency_unreliable" {
		t.Fatalf("TPS basis = %q, want latency_unreliable", item.TPSBasis)
	}
}

func TestEffectiveOutputTokensForTPS(t *testing.T) {
	tests := []struct {
		name          string
		dimensions    Dimensions
		counters      Counters
		explicitTotal bool
		want          uint64
	}{
		{
			name:       "openai reasoning is included in output",
			dimensions: Dimensions{Provider: "openai"},
			counters:   Counters{InputTokens: 10, OutputTokens: 50, ReasoningTokens: 10, TotalTokens: 60},
			want:       50,
		},
		{
			name:       "anthropic cache accounting does not imply separate reasoning",
			dimensions: Dimensions{Provider: "anthropic"},
			counters:   Counters{InputTokens: 100, OutputTokens: 50, ReasoningTokens: 20, CacheReadTokens: 30, CacheCreationTokens: 10, TotalTokens: 190},
			want:       50,
		},
		{
			name:       "vertex uses separate reasoning",
			dimensions: Dimensions{Provider: "vertex"},
			counters:   Counters{InputTokens: 10, OutputTokens: 20, ReasoningTokens: 5, TotalTokens: 35},
			want:       25,
		},
		{
			name:       "ai studio uses separate reasoning",
			dimensions: Dimensions{Provider: "aistudio"},
			counters:   Counters{InputTokens: 10, OutputTokens: 20, ReasoningTokens: 5, TotalTokens: 35},
			want:       25,
		},
		{
			name:       "antigravity uses separate reasoning",
			dimensions: Dimensions{Provider: "antigravity"},
			counters:   Counters{InputTokens: 10, OutputTokens: 20, ReasoningTokens: 5, TotalTokens: 35},
			want:       25,
		},
		{
			name:       "interactions uses separate reasoning",
			dimensions: Dimensions{Provider: "custom", ExecutorType: "InteractionsExecutor"},
			counters:   Counters{InputTokens: 10, OutputTokens: 20, ReasoningTokens: 5, TotalTokens: 35},
			want:       25,
		},
		{
			name:       "unknown reasoning greater than output is separate",
			dimensions: Dimensions{Provider: "custom"},
			counters:   Counters{InputTokens: 10, OutputTokens: 10, ReasoningTokens: 20, TotalTokens: 40},
			want:       30,
		},
		{
			name:          "unknown explicit total can prove separate reasoning",
			dimensions:    Dimensions{Provider: "custom"},
			counters:      Counters{InputTokens: 10, OutputTokens: 20, ReasoningTokens: 5, TotalTokens: 35},
			explicitTotal: true,
			want:          25,
		},
		{
			name:          "unknown explicit total can prove reasoning is included",
			dimensions:    Dimensions{Provider: "custom"},
			counters:      Counters{InputTokens: 10, OutputTokens: 20, ReasoningTokens: 5, TotalTokens: 30},
			explicitTotal: true,
			want:          20,
		},
		{
			name:       "filled total does not prove separate reasoning",
			dimensions: Dimensions{Provider: "custom"},
			counters:   Counters{InputTokens: 10, OutputTokens: 20, ReasoningTokens: 5, TotalTokens: 35},
			want:       20,
		},
		{
			name:       "token addition saturates",
			dimensions: Dimensions{Provider: "gemini"},
			counters:   Counters{OutputTokens: math.MaxUint64, ReasoningTokens: math.MaxUint64},
			want:       math.MaxUint64,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := effectiveOutputTokensForTPS(test.dimensions, test.counters, test.explicitTotal); got != test.want {
				t.Fatalf("effective output tokens = %d, want %d", got, test.want)
			}
		})
	}
}

func TestRequestTPSWithoutGenerationTime(t *testing.T) {
	item := RequestDetail{
		Dimensions: Dimensions{Provider: "gemini"},
		Counters:   Counters{OutputTokens: 100, ReasoningTokens: 100},
	}
	if got := requestTPS(item, false); got != 0 {
		t.Fatalf("TPS = %v, want 0", got)
	}
}

func TestDecodeUsageTracksExplicitTotal(t *testing.T) {
	raw := []byte(`{"provider":"custom","detail":{"input_tokens":10,"output_tokens":20,"reasoning_tokens":5,"total_tokens":35}}`)
	usage, err := decodeUsage(raw, time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !usage.explicitTotalTokens {
		t.Fatal("explicit positive total was not recorded")
	}

	raw = []byte(`{"provider":"custom","detail":{"input_tokens":10,"output_tokens":20,"reasoning_tokens":5,"total_tokens":0}}`)
	usage, err = decodeUsage(raw, time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if usage.explicitTotalTokens {
		t.Fatal("filled total was incorrectly recorded as explicit")
	}
	if usage.Counters.TotalTokens != 35 {
		t.Fatalf("filled total = %d, want 35", usage.Counters.TotalTokens)
	}
}
