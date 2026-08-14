package service

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
)

// DetectResult is the result of platform detection.
//
// Platform/SiteType both hold the detected platform (SiteType mirrors the TS
// "siteType" naming; Platform is retained for backward compatibility with the
// existing /api/sites/detect consumers). Confidence is a coarse signal of how
// the platform was resolved (preset match = 1.0, hostname heuristic = 0.8).
// CanonicalURL is the normalized URL suitable for persistence/dedup.
type DetectResult struct {
	URL                    string  `json:"url"`
	CanonicalURL           string  `json:"canonicalUrl"`
	Platform               string  `json:"platform"`
	SiteType               string  `json:"siteType"`
	Confidence             float64 `json:"confidence"`
	InitializationPresetID *string `json:"initializationPresetId,omitempty"`
}

// DetectSite attempts to detect the platform for a given URL.
// When a site initialization preset matches the URL, the result uses the
// preset protocol family (openai/claude) and includes initializationPresetId.
// Otherwise falls back to hostname heuristics for vendor/platform tags.
func DetectSite(rawURL string) *DetectResult {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil
	}

	parsed, err := url.Parse(trimmed)
	// Allow scheme-less inputs such as "api.deepseek.com/v1" or
	// "api.sensetime.com": url.Parse succeeds but leaves Host empty for bare
	// host/path strings, so retry with an https:// prefix.
	if err != nil || parsed == nil || parsed.Host == "" {
		if withScheme, schemeErr := url.Parse("https://" + trimmed); schemeErr == nil && withScheme.Host != "" {
			parsed = withScheme
			err = nil
		}
	}
	if err != nil || parsed == nil || parsed.Host == "" {
		return nil
	}

	// CanonicalURL: normalize the URL for detection purposes
	canonicalURL := parsed.String()
	host := strings.ToLower(parsed.Hostname())

	// Prefer initialization presets (protocol family + preset id) when the
	// host/path rules match. This keeps createSite/detectSite aligned with the
	// frontend siteInitializationPresets registry.
	if preset := DetectSiteInitializationPreset(trimmed, ""); preset != nil {
		id := preset.ID
		return &DetectResult{
			URL:                    canonicalURL,
			CanonicalURL:           CanonicalizeSiteURL(trimmed),
			Platform:               preset.Platform,
			SiteType:               preset.Platform,
			Confidence:             1.0,
			InitializationPresetID: &id,
		}
	}

	// Heuristic detection based on hostname patterns
	// will replace this with real adapter-based detection.
	var platform string
	switch {
	case strings.Contains(host, "api.openai.com") || strings.Contains(host, "openai.com"):
		platform = "openai"
	case strings.Contains(host, "api.anthropic.com") || strings.Contains(host, "anthropic.com"):
		platform = "anthropic"
	case strings.Contains(host, "generativelanguage.googleapis.com") || strings.Contains(host, "ai.google.dev"):
		platform = "gemini"
	case strings.Contains(host, "api.deepseek.com") || strings.Contains(host, "deepseek.com"):
		platform = "deepseek"
	case strings.Contains(host, "api.moonshot.cn") || strings.Contains(host, "moonshot.cn"):
		platform = "moonshot"
	case strings.Contains(host, "dashscope.aliyuncs.com") || strings.Contains(host, "dashscope"):
		platform = "dashscope"
	case strings.Contains(host, "api.baichuan-ai.com"):
		platform = "baichuan"
	case strings.Contains(host, "api.zhipuai.cn") || strings.Contains(host, "bigmodel.cn"):
		platform = "zhipu"
	case strings.Contains(host, "api.minimax.chat") || strings.Contains(host, "minimax"):
		platform = "minimax"
	case strings.Contains(host, "api.stepfun.com") || strings.Contains(host, "stepfun"):
		platform = "stepfun"
	case strings.Contains(host, "ark.cn-beijing.volces.com") || strings.Contains(host, "volcengine.com"):
		platform = "bytedance"
	case strings.Contains(host, "api.siliconflow.cn") || strings.Contains(host, "siliconflow"):
		platform = "siliconflow"
	case strings.Contains(host, "api-inference.modelscope.cn"):
		platform = "modelscope"
	case strings.Contains(host, "api.mistral.ai"):
		platform = "mistral"
	case strings.Contains(host, "api.cohere.ai") || strings.Contains(host, "cohere.com"):
		platform = "cohere"
	case strings.Contains(host, "api.together.xyz") || strings.Contains(host, "together.xyz"):
		platform = "together"
	case strings.Contains(host, "api.fireworks.ai"):
		platform = "fireworks"
	case strings.Contains(host, "api.groq.com"):
		platform = "groq"
	case strings.Contains(host, "api.perplexity.ai"):
		platform = "perplexity"
	case strings.Contains(host, "api.x.ai") || strings.Contains(host, "x.ai"):
		platform = "grok"
	case strings.Contains(host, "api-inference.huggingface.co") || strings.Contains(host, "hf.space"):
		platform = "huggingface"
	case strings.Contains(host, "azure.com") || strings.Contains(host, "openai.azure.com"):
		platform = "azure"
	case strings.Contains(host, ".github.com") && (strings.Contains(parsed.Path, "openai") || strings.Contains(parsed.Path, "copilot")):
		platform = "github-copilot"
	case strings.Contains(host, "claude.ai"):
		platform = "claude"
	case strings.Contains(host, "api.sensetime.com") || strings.Contains(host, "sensetime.com"):
		platform = "sensetime"
	case strings.Contains(host, "aistudio.google.com") || strings.Contains(host, "makersuite.google.com"):
		platform = "gemini"
	default:
		// Adapter-based detection for gateway forks (NewAPI/OneAPI/Sub2API/
		// one-hub/done-hub/veloera/cliproxyapi/anyrouter). Keyword heuristics
		// remain as a fallback for WAF/shield-fronted deployments whose HTTP
		// probes are blocked.
		if name := detectPlatformByAdapter(trimmed); name != "" {
			platform = name
		} else if strings.Contains(host, "anyrouter") || strings.Contains(parsed.Path, "anyrouter") {
			platform = "anyrouter"
		} else if strings.Contains(host, "oneapi") || strings.Contains(host, "new-api") || strings.Contains(host, "newapi") {
			platform = "new-api"
		}
	}

	if platform == "" {
		return nil
	}

	result := &DetectResult{
		URL:          canonicalURL,
		CanonicalURL: CanonicalizeSiteURL(trimmed),
		Platform:     platform,
		SiteType:     platform,
		Confidence:   0.8,
	}
	// Optional: attach a preset when hostname heuristics already chose a vendor
	// platform and a defaultUrl fallback matches under that protocol family.
	// Prefer the detected protocol family when the heuristic platform itself is
	// already openai/claude; otherwise leave preset empty (frontend can still
	// call DetectSiteInitializationPreset client-side).
	if platform == "openai" || platform == "claude" {
		if preset := DetectSiteInitializationPreset(trimmed, platform); preset != nil {
			id := preset.ID
			result.InitializationPresetID = &id
		}
	}
	return result
}

// CanonicalizeSiteURL returns a canonical URL for persistence.
// Mirrors TS analyzePrimarySiteUrl().persistedUrl behavior.
func CanonicalizeSiteURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return strings.TrimRight(trimmed, "/")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

// adapterDetectTimeout bounds the adapter-chain detection. Detection is a
// one-shot admin action (site create/import), so a few seconds is acceptable.
const adapterDetectTimeout = 6 * time.Second

// detectPlatformByAdapter runs the adapter registry Detect chain in registration
// order (specific forks before generic adapters, OneAPI catch-all last). The
// first true result wins. Errors are treated as "not detected" so one broken
// probe cannot abort the whole chain.
func detectPlatformByAdapter(rawURL string) string {
	ctx, cancel := context.WithTimeout(context.Background(), adapterDetectTimeout)
	defer cancel()

	for _, adapter := range platform.ListAdapters() {
		ok, err := adapter.Detect(ctx, rawURL)
		if err != nil {
			continue
		}
		if ok {
			return adapter.PlatformName()
		}
	}
	return ""
}
