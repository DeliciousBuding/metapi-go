package proxyhandler

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/prompt_filter"
)

// promptFilterMu guards the cached prompt filter instance and the signature it
// was built from. The filter is immutable after construction (regexes are
// pre-compiled), so a read lock on the hot path is sufficient.
var (
	promptFilterMu      sync.RWMutex
	promptFilterCache   *promptfilter.Filter
	promptFilterSig     string
	promptFilterBuildOK bool
)

// getPromptFilter returns the cached prompt safety filter, or nil when it is
// disabled (PROMPT_FILTER_ENABLED=false) or config has not been loaded yet.
//
// The filter is rebuilt only when the runtime deny-patterns signature changes,
// so the compiled regexes are reused across requests. A build failure logs
// once and disables the filter (fail-open: never block legit traffic because
// of a bad pattern).
func getPromptFilter() *promptfilter.Filter {
	cfg := config.GetSafe()
	if cfg == nil || !cfg.PromptFilterEnabled {
		return nil
	}
	sig := strings.Join(cfg.PromptFilterDenyPatterns, "\x00")

	promptFilterMu.RLock()
	if promptFilterCache != nil && promptFilterSig == sig {
		f := promptFilterCache
		promptFilterMu.RUnlock()
		return f
	}
	promptFilterMu.RUnlock()

	promptFilterMu.Lock()
	defer promptFilterMu.Unlock()
	// Double-check after acquiring the write lock.
	if promptFilterCache != nil && promptFilterSig == sig {
		return promptFilterCache
	}
	f, err := promptfilter.NewFilter(cfg.PromptFilterDenyPatterns)
	if err != nil {
		// Log only when the signature changes to avoid spamming on every request.
		if !promptFilterBuildOK || promptFilterSig != sig {
			slog.Error("prompt filter build failed; safety filter disabled (fail-open)",
				"error", err)
		}
		promptFilterCache = nil
		promptFilterSig = sig
		promptFilterBuildOK = false
		return nil
	}
	promptFilterCache = f
	promptFilterSig = sig
	promptFilterBuildOK = true
	return f
}

// resetPromptFilterForTests clears the cached filter so unit tests can toggle
// config flags and force a rebuild. Production code never calls this.
func resetPromptFilterForTests() {
	promptFilterMu.Lock()
	defer promptFilterMu.Unlock()
	promptFilterCache = nil
	promptFilterSig = ""
	promptFilterBuildOK = false
}

// checkPromptFilter runs the prompt safety filter on the parsed request body.
// When the request is blocked it returns a non-nil SurfResult describing the
// 403 response; otherwise it returns nil and the caller proceeds upstream.
//
// Privacy: only the matched pattern name and requested model are logged —
// never the prompt content. The block happens before any upstream forwarding
// so streaming requests never start mid-stream.
func checkPromptFilter(ctx *Ctx) *SurfResult {
	f := getPromptFilter()
	if f == nil {
		return nil
	}
	blocked, reason := f.Check(ctx.Body)
	if !blocked {
		return nil
	}
	slog.Warn("prompt blocked by safety filter",
		"reason", reason,
		"model", ctx.RequestedModel,
		"surface", ctx.SurfaceFormat,
	)
	return &SurfResult{
		OK:        false,
		Status:    http.StatusForbidden,
		Error:     "Prompt blocked by safety filter: " + reason,
		ErrorType: "safety_filter",
	}
}
