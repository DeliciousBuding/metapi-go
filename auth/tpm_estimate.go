package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/deliciousbuding/metapi-go/internal/tokenestimate"
)

// Best-effort TPM admission estimate.

// Prefer recognized JSON inputs and their requested output budget. For a
// truncated or unrecognized envelope, use the byte-count fallback below.

// Never invent large defaults that false-block admission.
const (
	tpmEstimateMaxTokens int64 = 128_000
	// Cap how much of the body we peek for estimation so huge uploads do not
	// blow middleware memory. Content-Length floor still applies above the cap.
	tpmEstimateMaxBodyBytes int64 = 256 << 10 // 256 KiB
)

// estimateAdmissionTokens returns a best-effort token estimate for TPM admission.
// Safe to call on any request; restores r.Body when it was peeked.
// Returns at least 1 so callers never skip TPM accounting for an admitted
// request; an unestimable body conservatively reserves the minimum.
func estimateAdmissionTokens(r *http.Request) int64 {
	if r == nil {
		return 1
	}

	readBytes := int64(0)
	if r.Body != nil && r.Body != http.NoBody {
		// Peek up to cap+1 so we can detect truncation without consuming the rest.
		peek, err := io.ReadAll(io.LimitReader(r.Body, tpmEstimateMaxBodyBytes+1))
		readBytes = int64(len(peek))
		if err != nil {
			// Re-chain whatever was read with the remaining body; skip JSON path.
			r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(peek), r.Body))
		} else if int64(len(peek)) > tpmEstimateMaxBodyBytes {
			// Partial peek only — re-attach full stream; do not parse partial JSON.
			r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(peek), r.Body))
		} else {
			// Full body (within cap) was consumed; restore from bytes.
			r.Body = io.NopCloser(bytes.NewReader(peek))
			var body map[string]any
			if json.Unmarshal(peek, &body) == nil {
				if n := tokenestimate.Request(body, tpmEstimateMaxTokens); n > 0 {
					return n
				}
			}
		}
	}

	chars := max(readBytes, r.ContentLength)

	if chars <= 0 {
		// No signal at all: reserve the 1-token minimum rather than skipping.
		return 1
	}

	// chars/4 ≈ tokens; clamp positive results to [1, 128000].
	tokens := chars / 4
	if tokens < 1 {
		tokens = 1
	}
	if tokens > tpmEstimateMaxTokens {
		tokens = tpmEstimateMaxTokens
	}
	return tokens
}
