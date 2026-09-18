package plugin

import (
	"fmt"
	"strings"
	"time"
)

const (
	defaultRequestPageSize = 100
	maxRequestPageSize     = 500

	bufferedStreamMaxGenerationNS = uint64(time.Second)
	bufferedStreamMinTokens       = uint64(200)
	bufferedStreamMaxTPS          = 500.0
)

// RequestDetail contains metadata and usage counters for one model request.
// Prompt and response content are intentionally never persisted.
type RequestDetail struct {
	Sequence uint64    `json:"sequence"`
	Time     time.Time `json:"time"`
	Dimensions
	Counters
	Result        string         `json:"result"`
	LatencyNS     uint64         `json:"latency_ns"`
	TTFTNS        uint64         `json:"ttft_ns"`
	GenerationNS  uint64         `json:"generation_ns"`
	TPS           float64        `json:"tps"`
	TPSBasis      string         `json:"tps_basis,omitempty"`
	CacheHit      bool           `json:"cache_hit"`
	EstimatedCost *EstimatedCost `json:"estimated_cost,omitempty"`
}

type RequestPage struct {
	GeneratedAt       time.Time       `json:"generated_at"`
	Range             string          `json:"range"`
	PriceBookRevision uint64          `json:"price_book_revision"`
	Total             int             `json:"total"`
	Offset            int             `json:"offset"`
	Limit             int             `json:"limit"`
	Items             []RequestDetail `json:"items"`
}

func (p *RequestPage) Redact() {
	for i := range p.Items {
		p.Items[i].Dimensions.Redact()
	}
}

func (p *RequestPage) Reveal(decrypt DecryptFunc) {
	for i := range p.Items {
		p.Items[i].Dimensions.Reveal(decrypt)
	}
}

type reasoningTokenAccounting uint8

const (
	reasoningAccountingUnknown reasoningTokenAccounting = iota
	reasoningAccountingIncludedInOutput
	reasoningAccountingSeparateFromOutput
)

// reasoningAccountingForDimensions mirrors the provider semantics used by
// CLIProxyAPI to normalize token accounting. It only determines whether the
// reasoning bucket is already included in the output bucket.
func reasoningAccountingForDimensions(dimensions Dimensions) reasoningTokenAccounting {
	provider := strings.ToLower(strings.TrimSpace(dimensions.Provider))
	executor := strings.ToLower(strings.TrimSpace(dimensions.ExecutorType))
	value := provider + " " + executor
	if value == " " || value == "unknown" || value == "unknown unknown" {
		return reasoningAccountingUnknown
	}
	if executor == "openaicompatexecutor" ||
		provider == "openai-compatibility" ||
		strings.HasPrefix(provider, "openai-compatible-") {
		return reasoningAccountingIncludedInOutput
	}
	if strings.Contains(value, "claude") || strings.Contains(value, "anthropic") {
		return reasoningAccountingIncludedInOutput
	}
	for _, marker := range []string{"gemini", "aistudio", "antigravity", "vertex", "interaction"} {
		if strings.Contains(value, marker) {
			return reasoningAccountingSeparateFromOutput
		}
	}
	for _, marker := range []string{"openai", "codex", "xai", "grok", "kimi", "qwen", "deepseek", "openrouter"} {
		if strings.Contains(value, marker) {
			return reasoningAccountingIncludedInOutput
		}
	}
	return reasoningAccountingUnknown
}

// effectiveOutputTokensForTPS returns the token numerator for TPS. Known
// protocol semantics take precedence. For unknown providers, only unambiguous
// arithmetic evidence may add reasoning tokens; otherwise the historical
// output-only numerator is retained.
func effectiveOutputTokensForTPS(dimensions Dimensions, counters Counters, explicitTotal bool) uint64 {
	if counters.ReasoningTokens == 0 {
		return counters.OutputTokens
	}
	switch reasoningAccountingForDimensions(dimensions) {
	case reasoningAccountingIncludedInOutput:
		return counters.OutputTokens
	case reasoningAccountingSeparateFromOutput:
		return saturatingAdd(counters.OutputTokens, counters.ReasoningTokens)
	}

	if counters.ReasoningTokens > counters.OutputTokens {
		return saturatingAdd(counters.OutputTokens, counters.ReasoningTokens)
	}
	if !explicitTotal {
		return counters.OutputTokens
	}
	inputAndOutput := saturatingAdd(counters.InputTokens, counters.OutputTokens)
	inputOutputAndReasoning := saturatingAdd(inputAndOutput, counters.ReasoningTokens)
	if counters.TotalTokens == inputOutputAndReasoning {
		return saturatingAdd(counters.OutputTokens, counters.ReasoningTokens)
	}
	return counters.OutputTokens
}

func requestTPSWithBasis(item RequestDetail, explicitTotal bool) (float64, string) {
	if item.GenerationNS == 0 {
		return 0, ""
	}
	outputTokens := effectiveOutputTokensForTPS(item.Dimensions, item.Counters, explicitTotal)
	basis := "generation"
	denominator := item.GenerationNS
	if likelyBufferedStream(item, outputTokens) {
		basis = "latency_buffered"
		denominator = item.LatencyNS
	}
	if denominator == 0 {
		return 0, basis
	}
	tps := float64(outputTokens) / (float64(denominator) / float64(time.Second))
	if basis == "latency_buffered" && tps > bufferedStreamMaxTPS {
		// A short total latency cannot be a trustworthy estimate of model
		// generation time either. Do not replace one explosive value with
		// another; expose the measurement as unavailable instead.
		return 0, "latency_unreliable"
	}
	return tps, basis
}

func requestTPS(item RequestDetail, explicitTotal bool) float64 {
	tps, _ := requestTPSWithBasis(item, explicitTotal)
	return tps
}

func likelyBufferedStream(item RequestDetail, outputTokens uint64) bool {
	if reasoningAccountingForDimensions(item.Dimensions) != reasoningAccountingSeparateFromOutput ||
		item.TTFTNS == 0 || item.LatencyNS == 0 || item.GenerationNS == 0 || item.GenerationNS > bufferedStreamMaxGenerationNS ||
		item.LatencyNS < item.GenerationNS {
		return false
	}
	rawTPS := float64(outputTokens) / (float64(item.GenerationNS) / float64(time.Second))
	return outputTokens > bufferedStreamMinTokens || rawTPS > bufferedStreamMaxTPS
}

func requestDetailForUsage(usage normalizedUsage, sequence uint64) RequestDetail {
	generationNS := usage.LatencyNS
	if usage.TTFTNS > 0 && usage.LatencyNS >= usage.TTFTNS {
		generationNS = usage.LatencyNS - usage.TTFTNS
	}
	result := "成功"
	if usage.Dimensions.Failed {
		result = "失败"
		if usage.Dimensions.FailureStatus > 0 {
			result = fmt.Sprintf("失败 (HTTP %d)", usage.Dimensions.FailureStatus)
		}
	}
	item := RequestDetail{
		Sequence:     sequence,
		Time:         usage.RequestedAt.UTC(),
		Dimensions:   usage.Dimensions,
		Counters:     usage.Counters,
		Result:       result,
		LatencyNS:    usage.LatencyNS,
		TTFTNS:       usage.TTFTNS,
		GenerationNS: generationNS,
		CacheHit:     usage.Counters.CacheReadTokens > 0,
	}
	item.TPS, item.TPSBasis = requestTPSWithBasis(item, usage.explicitTotalTokens)
	return item
}
