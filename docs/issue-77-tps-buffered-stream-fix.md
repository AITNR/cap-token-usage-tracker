# Issue #77: Buffered-stream TPS correction

- Issue: https://github.com/AITNR/cap-token-usage-tracker/issues/77
- Scope: per-request TPS, historical query recalculation, dashboard status, and CSV export
- Storage: no schema change and no rewrite or migration of historical request records

## Root cause

Per-request TPS originally used latency_ns - ttft_ns as its denominator. For separate-reasoning protocols such as Gemini, Vertex, AI Studio, Antigravity, and Interactions, the upstream or proxy can buffer data while the model is thinking. TTFT then contains most of the model generation time, while the post-first-token window measures only the final buffered transfer. Dividing output_tokens + reasoning_tokens by that short window produces false values in the thousands of TPS.

## Fix behavior

1. Keep protocol-aware token accounting: separate-reasoning protocols use output_tokens + reasoning_tokens; protocols that include reasoning in output continue to use output_tokens.
2. For separate-reasoning requests with generation_ns <= 1s, mark a request as likely buffered when output exceeds 200 tokens or the raw TPS exceeds 500.
3. Calculate likely buffered requests with full latency_ns and return tps_basis = latency_buffered.
4. If the full-latency result still exceeds 500 TPS, return tps = 0 and tps_basis = latency_unreliable. The dashboard shows an unavailable marker and CSV leaves TPS blank.
5. Recalculate TPS while serving /requests, so historical records are corrected without a bbolt migration.
6. Append a TPS basis column to CSV so exported values retain their calculation context.

## Verification

Coverage includes the Issue example, normal separate-reasoning streams, short-latency high-token requests, provider classification, historical query recalculation, dashboard script contracts, and all locale CSV headers.
