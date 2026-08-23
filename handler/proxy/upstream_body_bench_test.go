package proxyhandler

import (
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/proxy"
)

// benchBuildChatBody returns a syntactically valid streaming chat/completions
// JSON body of approximately size bytes (model first, large messages array).
func benchBuildChatBody(size int) []byte {
	content := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 6)
	msg := `{"role":"user","content":"` + content + `"},`
	var sb strings.Builder
	sb.WriteString(`{"model":"gpt-4o","stream":true,"messages":[`)
	for sb.Len() < size {
		sb.WriteString(msg)
	}
	s := strings.TrimSuffix(sb.String(), ",")
	s += `],"temperature":0.7}`
	return []byte(s)
}

var benchSizes = map[string]int{"2KB": 2 << 10, "200KB": 200 << 10, "2MB": 2 << 20}

// BenchmarkSwapModelInJSON_NoMapping covers the dominant production case: the
// selected channel does not remap the model, so the body already carries the
// target model value.
func BenchmarkSwapModelInJSON_NoMapping(b *testing.B) {
	for _, name := range []string{"2KB", "200KB", "2MB"} {
		body := benchBuildChatBody(benchSizes[name])
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				_ = swapModelInJSON(body, "gpt-4o")
			}
		})
	}
}

// BenchmarkSwapModelInJSON_Mapped covers real model_mapping: the upstream model
// differs from the body's model and a rewrite is unavoidable.
func BenchmarkSwapModelInJSON_Mapped(b *testing.B) {
	for _, name := range []string{"2KB", "200KB", "2MB"} {
		body := benchBuildChatBody(benchSizes[name])
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				_ = swapModelInJSON(body, "gpt-4o-2024-11-20")
			}
		})
	}
}

// BenchmarkUpstreamBodyPipeline_NoMapping mirrors the per-attempt body
// construction of dispatchSelectedUpstream for a streaming chat request with no
// model mapping: model swap, responses/continuity sanitize gate, site stream
// preference, and include_usage injection.
func BenchmarkUpstreamBodyPipeline_NoMapping(b *testing.B) {
	for _, name := range []string{"2KB", "200KB", "2MB"} {
		body := benchBuildChatBody(benchSizes[name])
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for i := 0; i < b.N; i++ {
				swapped := swapModelInJSON(body, "gpt-4o")
				sanitized, err := sanitizeUpstreamJSONBody(swapped, "openai", "/v1/chat/completions", "gpt-4o")
				if err != nil {
					b.Fatal(err)
				}
				streamed, _ := applyUpstreamStreamPreference(sanitized, "openai", "/v1/chat/completions", proxy.SiteProtocolPreference{})
				_, _ = applyUpstreamStreamIncludeUsage(streamed, "openai", "/v1/chat/completions", true)
			}
		})
	}
}
