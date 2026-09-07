package proxy

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

func TestJudgeUpstreamContent_ExplicitErrorEvent(t *testing.T) {
	prevRt := config.RuntimeSafe()
	t.Cleanup(func() { config.SetRuntime(prevRt) })

	errorVerdict := UpstreamVerdict{
		Failed: true, Code: FailureCodeErrorEvent, Status: 502,
		Reason: "Upstream returned an error event",
	}
	for _, tc := range []struct {
		name                  string
		runtime               *config.RuntimeSettings
		hasError, hasOutput   bool
		unreadable, withUsage bool
		want                  UpstreamVerdict
	}{
		{
			name: "default settings", runtime: &config.RuntimeSettings{},
			hasError: true, want: errorVerdict,
		},
		{
			name: "no runtime snapshot", hasError: true, want: errorVerdict,
		},
		{
			name: "output and usage do not erase an error", runtime: &config.RuntimeSettings{},
			hasError: true, hasOutput: true, withUsage: true, want: errorVerdict,
		},
		{
			name: "explicit error before empty heuristic", runtime: &config.RuntimeSettings{ProxyEmptyContentFailEnabled: true},
			hasError: true, want: errorVerdict,
		},
		{
			name: "configured keyword retains its existing verdict", runtime: &config.RuntimeSettings{ProxyErrorKeywords: []string{"fixture"}},
			hasError: true,
			want:     UpstreamVerdict{Failed: true, Code: FailureCodeErrorKeyword, Status: 502, Reason: "Upstream response matched failure keyword: fixture"},
		},
		{
			name: "unreadable body is not evidence", runtime: &config.RuntimeSettings{},
			hasError: true, unreadable: true,
		},
		{
			name: "ordinary output mentioning error", runtime: &config.RuntimeSettings{},
			hasOutput: true,
		},
		{
			name: "no runtime and no explicit error", hasOutput: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.SetRuntime(tc.runtime)
			facts := UpstreamContentFacts{
				StatusCode: 200, Streaming: true,
				RawText:       "fixture text mentioning error",
				HasErrorEvent: tc.hasError, HasOutput: tc.hasOutput, Unreadable: tc.unreadable,
			}
			if tc.withUsage {
				facts.Usage = &UsageSummary{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}
			}
			if got := JudgeUpstreamContent(facts); got != tc.want {
				t.Fatalf("verdict = %+v, want %+v", got, tc.want)
			}
		})
	}
}
