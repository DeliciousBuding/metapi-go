package backup

import "testing"

func TestAxonHubNativeDomesticChatProfiles(t *testing.T) {
	for _, provider := range []string{"deepseek", "zai", "zhipu", "xiaomi"} {
		for _, custom := range []bool{false, true} {
			channel := AxonHubSourceChannel{ID: 1, Type: provider, BaseURL: "https://fixture.invalid", Credentials: AxonHubSourceCredentials{APIKey: "fixture"}, SupportedModels: []string{"model"}}
			if custom {
				channel.Endpoints = []AxonHubSourceEndpoint{{APIFormat: "openai/chat_completions", Path: "/custom/chat"}}
			}
			compiled, reasons, _ := compileAxonHubChannel(channel)
			if len(reasons) > 0 {
				t.Fatal(reasons)
			}
			want := "zai"
			if provider == "deepseek" {
				want = "deepseek"
			}
			if custom {
				want = ""
			}
			if compiled.Endpoints.Chat.Profile != want {
				t.Fatalf("%s custom=%v profile=%q want=%q", provider, custom, compiled.Endpoints.Chat.Profile, want)
			}
		}
	}
}
