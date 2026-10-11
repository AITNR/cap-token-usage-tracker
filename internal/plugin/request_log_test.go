package plugin

import (
	"encoding/json"
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

func TestRequestEndToEndTPSUsesHostLatencyDenominator(t *testing.T) {
	usage := normalizedUsage{
		Dimensions:          Dimensions{Provider: "gemini", Model: "gemini-thinking"},
		RequestedAt:         time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
		LatencyNS:           uint64(2250 * time.Millisecond),
		TTFTNS:              uint64(250 * time.Millisecond),
		Stream:              true,
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
	// The streaming numerator is unchanged: output + separate reasoning over the
	// post-first-token window.
	if item.TPS != 325 {
		t.Fatalf("TPS = %v, want 325", item.TPS)
	}
	if !item.Stream {
		t.Fatal("stream flag was not persisted from the host record")
	}
	// End-to-end TPS shares the numerator but divides by total latency, so it can
	// never exceed the streaming rate.
	const wantEndToEnd = 650.0 / 2.25
	if diff := item.EndToEndTPS - wantEndToEnd; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("EndToEndTPS = %v, want %v", item.EndToEndTPS, wantEndToEnd)
	}
	if item.EndToEndTPS >= item.TPS {
		t.Fatalf("end-to-end TPS %v must not exceed streaming TPS %v", item.EndToEndTPS, item.TPS)
	}
}

func TestRequestEndToEndTPSWithoutLatency(t *testing.T) {
	item := RequestDetail{
		Dimensions: Dimensions{Provider: "gemini"},
		Counters:   Counters{OutputTokens: 100, ReasoningTokens: 100},
	}
	if got := requestEndToEndTPS(item, false); got != 0 {
		t.Fatalf("EndToEndTPS = %v, want 0", got)
	}
}

func TestRequestEndToEndTPSWithoutTTFTStillComputes(t *testing.T) {
	// A non-streaming or TTFT-less record still has a usable end-to-end rate,
	// even though the streaming TPS falls back to the same denominator.
	item := RequestDetail{
		Dimensions:   Dimensions{Provider: "openai-compatible-demo"},
		LatencyNS:    uint64(10 * time.Second),
		GenerationNS: uint64(10 * time.Second),
		Counters:     Counters{OutputTokens: 200, TotalTokens: 200},
	}
	if got := requestEndToEndTPS(item, false); got != 20 {
		t.Fatalf("EndToEndTPS = %v, want 20", got)
	}
}

func TestDecodeUsageReadsStreamFlag(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		raw  string
		want bool
	}{
		{name: "stream true", raw: `{"provider":"custom","Stream":true}`, want: true},
		{name: "stream false", raw: `{"provider":"custom","Stream":false}`, want: false},
		{name: "snake case alias", raw: `{"provider":"custom","stream":true}`, want: true},
		{name: "absent field is not stream", raw: `{"provider":"custom"}`, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			usage, err := decodeUsage([]byte(test.raw), now)
			if err != nil {
				t.Fatal(err)
			}
			if usage.Stream != test.want {
				t.Fatalf("stream = %v, want %v", usage.Stream, test.want)
			}
		})
	}
}

func TestFirstBoolPresentDistinguishesAbsentFromFalse(t *testing.T) {
	root := map[string]json.RawMessage{}
	value, ok := firstBoolPresent(root, "Stream")
	if ok || value {
		t.Fatalf("absent field reported value=%v present=%v", value, ok)
	}
	root["Stream"] = json.RawMessage(`false`)
	value, ok = firstBoolPresent(root, "Stream")
	if !ok || value {
		t.Fatalf("explicit false reported value=%v present=%v", value, ok)
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
