package backup

import (
	"sort"
	"strings"
	"testing"
)

// axonHubAuditedChannelTypes is a frozen copy of AxonHub's channel.Type enum
// (internal/ent/channel/channel.go) at axonhub commit
// e863c6fe1942deddd0f6e471fa003c430e5314f0. Adding a provider upstream must
// extend both this list and axonHubProviderTypes, which turns "an AxonHub
// upgrade introduced a provider nobody reviewed" into a red test instead of a
// mis-mapped channel.
var axonHubAuditedChannelTypes = []string{
	"openai", "openai_responses", "atlascloud", "cline", "codex", "vercel", "anthropic",
	"anthropic_aws", "anthropic_gcp", "gemini_openai", "gemini", "gemini_vertex", "deepseek",
	"deepseek_anthropic", "deepinfra", "qiniu", "fireworks", "doubao", "doubao_anthropic",
	"moonshot", "moonshot_anthropic", "zhipu", "zai", "zhipu_anthropic", "zai_anthropic",
	"anthropic_fake", "openai_fake", "openrouter", "xiaomi", "xiaomi_anthropic", "xai", "ppio",
	"siliconflow", "volcengine", "volcengine_anthropic", "longcat", "longcat_anthropic", "minimax",
	"minimax_anthropic", "aihubmix", "aihubmix_anthropic", "burncloud", "modelscope", "bailian",
	"bailian_anthropic", "moonshot_coding", "jina", "github", "github_copilot", "claudecode",
	"cerebras", "antigravity", "nanogpt", "nanogpt_responses", "opencode_go", "opencode_go_anthropic",
	"ollama", "ollama_anthropic", "evolink", "evolink_anthropic", "groq", "qiniu_anthropic", "fenno",
}

func TestAxonHubProviderTableCoversAuditedEnum(t *testing.T) {
	audited := map[string]bool{}
	for _, name := range axonHubAuditedChannelTypes {
		if audited[name] {
			t.Fatalf("audited type list repeats %q", name)
		}
		audited[name] = true
		spec, ok := axonHubProviderTypes[name]
		if !ok {
			t.Errorf("provider %q is not classified; add it to axonHubProviderTypes", name)
			continue
		}
		if spec.Supported && spec.Protocols == 0 {
			t.Errorf("provider %q is marked supported but carries no default protocol", name)
		}
		if !spec.Supported && spec.Reason == "" {
			t.Errorf("provider %q is refused without a reason", name)
		}
		if spec.Supported && spec.Reason != "" {
			t.Errorf("provider %q is supported and also refused", name)
		}
	}
	var unlisted []string
	for name := range axonHubProviderTypes {
		if !audited[name] {
			unlisted = append(unlisted, name)
		}
	}
	sort.Strings(unlisted)
	if len(unlisted) > 0 {
		t.Fatalf("provider table has types outside the audited enum: %v", unlisted)
	}
}

func TestCompileAxonHubPlanResolvesEveryAssociationKind(t *testing.T) {
	src, err := ParseAxonHubSource(readAxonHubFixture(t, "axonhub-associations.json"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.channels) != 3 {
		t.Fatalf("compiled channels = %d, want 3 servable channels", len(plan.channels))
	}
	if len(plan.skipped) != 1 || plan.skipped[0].Type != "gemini" {
		t.Fatalf("unservable provider was not reported: %#v", plan.skipped)
	}
	if got := plan.skipped[0].Reasons; len(got) != 1 || got[0] != reasonProviderTranslation {
		t.Fatalf("gemini skip reasons = %#v", got)
	}

	byPattern := map[string]axonHubPlanRoute{}
	for _, route := range plan.routes {
		byPattern[route.Pattern] = route
	}
	for _, pattern := range []string{"gpt-5", "claude-sonnet-4.5", "deepseek-v4", "legacy-alias", "gpt-family"} {
		if _, ok := byPattern[pattern]; !ok {
			t.Fatalf("route %q was not compiled; routes=%v", pattern, routePatterns(plan))
		}
	}
	if _, ok := byPattern["gemini-3-pro"]; ok {
		t.Fatal("a route was compiled onto an unservable provider")
	}
	if _, ok := byPattern["orphan-model"]; ok {
		t.Fatal("a model with no association produced a route")
	}
	// channel_model on a channel with two API keys round-robins into two grants.
	if got := len(byPattern["gpt-5"].Items); got != 2 {
		t.Fatalf("gpt-5 candidate count = %d, want one per credential", got)
	}
	// The model association matches both the OpenAI channel (two credentials)
	// and the Anthropic channel (one credential).
	if got := len(byPattern["claude-sonnet-4.5"].Items); got != 3 {
		t.Fatalf("claude-sonnet-4.5 candidate count = %d, want 3", got)
	}
	// Route enablement mirrors the source *model*, while the disabled source
	// channel stays disabled: the two carry different AxonHub state and must
	// both survive the import.
	if !byPattern["deepseek-v4"].Enabled {
		t.Fatal("an enabled source model produced a disabled route")
	}
	disabledChannels := 0
	for _, channel := range plan.channels {
		if !channel.Enabled {
			disabledChannels++
		}
	}
	if disabledChannels != 1 {
		t.Fatalf("disabled source channels = %d, want 1", disabledChannels)
	}
	if !residualContains(plan.residuals, "manual_models_not_imported_until_synced") {
		t.Fatalf("manual models were not reported as a residual: %#v", plan.residuals)
	}
	if !residualContains(plan.residuals, "declared_protocol_not_servable:openai/embeddings") {
		t.Fatalf("declared unservable protocol was not reported: %#v", plan.residuals)
	}
	if !residualContains(plan.residuals, "model_has_no_importable_channel:gemini-3-pro") {
		t.Fatalf("unreachable model was not reported: %#v", plan.residuals)
	}
	if plan.notImported["projects"] != 1 || plan.notImported["apiKeys"] != 1 || plan.notImported["modelPrices"] != 1 {
		t.Fatalf("unmanaged sections were not counted: %#v", plan.notImported)
	}
}

func TestCompileAxonHubPlanRefusesUnservableChannelShapes(t *testing.T) {
	cases := []struct {
		name    string
		channel string
		want    []string
	}{
		{
			name:    "unknown provider",
			channel: `{"id":1,"type":"typesafe","name":"x","base_url":"https://x.invalid","credentials":{"apiKey":"k"},"endpoints":[{"api_format":"openai/chat_completions"}]}`,
			want:    []string{"provider_type_unknown"},
		},
		{
			name:    "oauth credential",
			channel: `{"id":1,"type":"openai","name":"x","base_url":"https://x.invalid","credentials":{"oauth":{"access_token":"t"}},"endpoints":[{"api_format":"openai/chat_completions"}]}`,
			want:    []string{"oauth_credentials_unsupported", "credential_missing"},
		},
		{
			name:    "gcp credential",
			channel: `{"id":1,"type":"openai","name":"x","base_url":"https://x.invalid","credentials":{"apiKey":"k","gcp":{"projectID":"p"}},"endpoints":[{"api_format":"openai/chat_completions"}]}`,
			want:    []string{"gcp_credentials_unsupported"},
		},
		{
			name:    "no servable protocol",
			channel: `{"id":1,"type":"openai","name":"x","base_url":"https://x.invalid","credentials":{"apiKey":"k"},"endpoints":[{"api_format":"openai/embeddings"}]}`,
			want:    []string{"no_servable_protocol"},
		},
		{
			name:    "unknown protocol",
			channel: `{"id":1,"type":"openai","name":"x","base_url":"https://x.invalid","credentials":{"apiKey":"k"},"endpoints":[{"api_format":"typesafe/system_one"}]}`,
			want:    []string{"api_format_unsupported"},
		},
		{
			name:    "websocket transport",
			channel: `{"id":1,"type":"openai_responses","name":"x","base_url":"https://x.invalid","credentials":{"apiKey":"k"},"endpoints":[{"api_format":"openai/responses","transport":"websocket"}]}`,
			want:    []string{"websocket_transport_unsupported"},
		},
		{
			name:    "endpoint base url override",
			channel: `{"id":1,"type":"openai","name":"x","base_url":"https://x.invalid","credentials":{"apiKey":"k"},"endpoints":[{"api_format":"openai/chat_completions","base_url":"https://other.invalid"}]}`,
			want:    []string{"endpoint_base_url_override_unsupported"},
		},
		{
			name:    "body override operations",
			channel: `{"id":1,"type":"openai","name":"x","base_url":"https://x.invalid","credentials":{"apiKey":"k"},"settings":{"bodyOverrideOperations":[{"op":"set","path":"temperature","value":"0.1"}]},"endpoints":[{"api_format":"openai/chat_completions"}]}`,
			want:    []string{"channel_body_override_unsupported"},
		},
		{
			name:    "unknown settings key",
			channel: `{"id":1,"type":"openai","name":"x","base_url":"https://x.invalid","credentials":{"apiKey":"k"},"settings":{"futureKnob":true},"endpoints":[{"api_format":"openai/chat_completions"}]}`,
			want:    []string{"channel_settings_unsupported:futureKnob"},
		},
		{
			name:    "stream policy",
			channel: `{"id":1,"type":"openai","name":"x","base_url":"https://x.invalid","credentials":{"apiKey":"k"},"policies":{"stream":"require"},"endpoints":[{"api_format":"openai/chat_completions"}]}`,
			want:    []string{"channel_stream_policy_unsupported"},
		},
		{
			name:    "transform options",
			channel: `{"id":1,"type":"openai","name":"x","base_url":"https://x.invalid","credentials":{"apiKey":"k"},"settings":{"transformOptions":{"replaceDeveloperRoleWithSystem":true}},"endpoints":[{"api_format":"openai/chat_completions"}]}`,
			want:    []string{"channel_transform_options_unsupported"},
		},
		{
			name:    "forbidden metadata target",
			channel: `{"id":1,"type":"openai","name":"x","base_url":"http://169.254.169.254/latest","credentials":{"apiKey":"k"},"endpoints":[{"api_format":"openai/chat_completions"}]}`,
			want:    []string{"base_url_target_forbidden"},
		},
		{
			name:    "credential missing",
			channel: `{"id":1,"type":"openai","name":"x","base_url":"https://x.invalid","credentials":{},"endpoints":[{"api_format":"openai/chat_completions"}]}`,
			want:    []string{"credential_missing"},
		},
		{
			name:    "unknown credential kind",
			channel: `{"id":1,"type":"openai","name":"x","base_url":"https://x.invalid","credentials":{"apiKey":"k","managementApiKey":"m"},"endpoints":[{"api_format":"openai/chat_completions"}]}`,
			want:    []string{"credential_field_unsupported:managementApiKey"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{"version":"1.4","channels":[` + tc.channel + `],"models":[]}`
			src, err := ParseAxonHubSource([]byte(payload))
			if err != nil {
				t.Fatalf("ParseAxonHubSource: %v", err)
			}
			plan, err := CompileAxonHubPlan(src)
			if err != nil {
				t.Fatalf("CompileAxonHubPlan: %v", err)
			}
			if len(plan.skipped) != 1 {
				t.Fatalf("skipped channels = %#v, want the one unrepresentable channel", plan.skipped)
			}
			reasons := plan.skipped[0].Reasons
			for _, want := range tc.want {
				found := false
				for _, reason := range reasons {
					if reason == want {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("skip reasons %#v missing %q", reasons, want)
				}
			}
			if len(plan.channels) != 0 || len(plan.blocking) == 0 {
				t.Fatalf("a backup with no servable channel was not blocked: channels=%d blocking=%#v", len(plan.channels), plan.blocking)
			}
		})
	}
}

func TestCompileAxonHubPlanReportsSkipReasonsPerChannel(t *testing.T) {
	payload := `{"version":"1.4","channels":[
		{"id":1,"type":"openai","name":"ok","base_url":"https://ok.invalid","credentials":{"apiKey":"k"},"supported_models":["gpt-5"],"endpoints":[{"api_format":"openai/chat_completions"}]},
		{"id":2,"type":"gemini","name":"no","base_url":"https://no.invalid","credentials":{"apiKey":"k"},"endpoints":[{"api_format":"gemini/contents"}]}
	],"models":[{"id":1,"model_id":"gpt-5","type":"chat","status":"enabled","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"gpt-5"}}]}}]}`
	src, err := ParseAxonHubSource([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.skipped) != 1 || plan.skipped[0].SourceID != 2 {
		t.Fatalf("skipped channels = %#v", plan.skipped)
	}
	if len(plan.channels) != 1 || len(plan.routes) != 1 {
		t.Fatalf("servable channel was not compiled: channels=%d routes=%d", len(plan.channels), len(plan.routes))
	}
}

func TestAxonHubChannelEntriesMatchSourceSemantics(t *testing.T) {
	channel := AxonHubSourceChannel{
		SupportedModels: []string{"deepseek/deepseek-v4", "gpt-5"},
		Settings: AxonHubSourceChannelSettings{
			ExtraModelPrefix:        "lane",
			AutoTrimedModelPrefixes: []string{"deepseek"},
			ModelMappings:           []AxonHubSourceModelMapping{{From: "cheap-fast", To: "gpt-5"}},
		},
	}
	entries := axonHubChannelEntries(channel)
	for request, want := range map[string]string{
		"deepseek/deepseek-v4": "deepseek/deepseek-v4",
		"gpt-5":                "gpt-5",
		"lane/gpt-5":           "gpt-5",
		"deepseek-v4":          "deepseek/deepseek-v4",
		"cheap-fast":           "gpt-5",
	} {
		if got, ok := entries[request]; !ok || got != want {
			t.Errorf("entry %q = %q (present=%v), want %q", request, got, ok, want)
		}
	}

	channel.Settings.HideOriginalModels = true
	hidden := axonHubChannelEntries(channel)
	if _, ok := hidden["gpt-5"]; ok {
		t.Error("hideOriginalModels did not drop the direct entry")
	}
	if _, ok := hidden["cheap-fast"]; !ok {
		t.Error("hideOriginalModels dropped a mapped alias")
	}

	channel.Settings.HideOriginalModels = false
	channel.Settings.HideMappedModels = true
	hiddenMapped := axonHubChannelEntries(channel)
	if _, ok := hiddenMapped["gpt-5"]; ok {
		t.Error("hideMappedModels did not drop the direct path to the mapped target")
	}
	if _, ok := hiddenMapped["cheap-fast"]; !ok {
		t.Error("hideMappedModels dropped the mapping itself")
	}

	lowered := axonHubChannelEntries(AxonHubSourceChannel{
		SupportedModels: []string{"GPT-5"},
		Settings:        AxonHubSourceChannelSettings{LowercaseModelID: true},
	})
	if actual, ok := lowered["gpt-5"]; !ok || actual != "GPT-5" {
		t.Errorf("lowercaseModelId did not lowercase the request key only: %#v", lowered)
	}
}

func routePatterns(plan *axonHubPlan) []string {
	patterns := make([]string, 0, len(plan.routes))
	for _, route := range plan.routes {
		patterns = append(patterns, route.Pattern)
	}
	sort.Strings(patterns)
	return patterns
}

func residualContains(residuals []string, want string) bool {
	for _, residual := range residuals {
		if strings.Contains(residual, want) {
			return true
		}
	}
	return false
}
