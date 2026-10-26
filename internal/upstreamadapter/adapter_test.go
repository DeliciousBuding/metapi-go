package upstreamadapter_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/upstreamadapter"
	"github.com/deliciousbuding/metapi-go/store"
)

var profileContracts = []string{
	"", "bailian", "bedrock", "cerebras", "claudecode", "cline", "codex",
	"codex-alpha-search", "codex-image", "deepseek", "github-copilot",
	"jina-embeddings", "longcat", "moonshot", "nanogpt", "ollama",
	"ollama-messages", "openrouter", "vertex", "vertex-anthropic",
	"xai-subscription", "zai",
}

// This joins both production consumers: templates pass the persisted endpoint
// contract, then the selected definition actually prepares and projects a wire.
func TestDefinitionsConnectTemplatesToExecutableAdapters(t *testing.T) {
	if got := upstreamadapter.Profiles(); !reflect.DeepEqual(got, profileContracts) {
		t.Fatalf("profile coverage changed: got %v, want %v", got, profileContracts)
	}
	for _, profile := range profileContracts {
		t.Run(profile, func(t *testing.T) {
			t.Parallel()
			definition, ok := upstreamadapter.Lookup(profile)
			if !ok {
				t.Fatal("advertised adapter cannot be resolved")
			}
			templates := upstreamadapter.Templates(profile)
			if profile == "" {
				if templates != nil {
					t.Fatal("native passthrough must not prescribe a protocol")
				}
			} else if len(templates) == 0 {
				t.Fatal("provider adapter lacks executable defaults")
			}
			for _, template := range templates {
				if template.Profile != profile || !strings.HasPrefix(template.Path, "/") {
					t.Fatalf("template lost profile or relative path: %+v", template)
				}
				address := strings.NewReplacer("{project}", "fixture-project", "{region}", "us-central1").Replace(template.Path)
				encoded, err := json.Marshal(map[string]store.DirectEndpoint{
					template.Protocol: {URL: "https://provider.invalid" + address, Auth: template.Auth, Profile: template.Profile, ModelPath: template.ModelPath, RequestModel: template.RequestModel},
				})
				if err != nil {
					t.Fatal(err)
				}
				var persisted store.DirectEndpoints
				if err := persisted.Scan(encoded); err != nil || !persisted.IsConfigured() {
					t.Fatalf("template cannot become a persisted endpoint: %s (%v)", encoded, err)
				}
			}

			body := fixtureBody(profile)
			original := bytes.Clone(body)
			downstream := http.Header{"Authorization": {"Bearer untrusted-fixture"}, "Session-Id": {"fixture-session"}}
			before := downstream.Clone()
			prepared, err := definition.Prepare(upstreamadapter.Input{Body: body, Downstream: downstream, EndpointURL: "https://provider.invalid/wire", ChannelID: 42, OAuth: true, AccountID: "fixture-account", SessionNamespace: "fixture-key"})
			if err != nil || prepared == nil || !json.Valid(prepared.Body) {
				t.Fatalf("adapter cannot prepare its wire: %v", err)
			}
			if !bytes.Equal(body, original) || !reflect.DeepEqual(downstream, before) {
				t.Fatal("adapter mutated the caller's request or headers")
			}
			if prepared.Headers == nil || prepared.Query == nil || prepared.Headers.Get("Authorization") != "" {
				t.Fatal("wire must own mutable headers/query without taking downstream credentials")
			}
			wantStream := profile == "codex" || profile == "codex-image"
			if prepared.ForceStream != wantStream || prepared.StripToolPrefix != (profile == "claudecode") {
				t.Fatal("request-scoped streaming or OAuth tool contract changed")
			}
			for _, stream := range []bool{false, true} {
				field := "message"
				object := "chat.completion"
				if stream {
					field, object = "delta", "chat.completion.chunk"
				}
				response := []byte(`{"object":"` + object + `","created":9007199254740993,"choices":[{"index":0,"` + field + `":{"role":"assistant","content":"ok","reasoning_content":"thought","reasoning":"thought"},"finish_reason":"stop"}]}`)
				projected, err := definition.ProjectResponse(response, stream)
				if err != nil || !json.Valid(projected) || !bytes.Contains(projected, []byte("9007199254740993")) {
					t.Fatalf("response projection lost its contract: %s (%v)", projected, err)
				}
				if slices.Contains([]string{"cline", "openrouter", "cerebras"}, profile) && bytes.Contains(projected, []byte(`"reasoning":`)) {
					t.Fatal("registry bypassed provider-specific reasoning projection")
				}
			}
		})
	}
}

func fixtureBody(profile string) []byte {
	switch profile {
	case "codex", "codex-image":
		return []byte(`{"model":"fixture","input":"hello","max_output_tokens":32}`)
	case "claudecode", "vertex-anthropic", "bedrock", "ollama-messages":
		return []byte(`{"model":"fixture","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"tools":[{"name":"echo","input_schema":{"type":"object"}}]}`)
	case "vertex":
		return []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"seed":9007199254740993}}`)
	case "jina-embeddings":
		return []byte(`{"model":"fixture","input":["hello"]}`)
	case "ollama":
		return []byte(`{"model":"fixture","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"type":"object"}}}]}`)
	default:
		return []byte(`{"model":"fixture","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"type":"object"}}}],"opaque":9007199254740993}`)
	}
}

func TestTemplatesDoNotLeakMutableDefaults(t *testing.T) {
	for _, profile := range profileContracts[1:] {
		before := upstreamadapter.Templates(profile)
		changed := upstreamadapter.Templates(profile)
		changed[0].Path = "/different-instance"
		changed[0].Profile = "different-instance"
		if !reflect.DeepEqual(before, upstreamadapter.Templates(profile)) {
			t.Fatalf("%s defaults shared mutable state", profile)
		}
	}
}

func TestNativeAndUnknownProfilesStayDistinct(t *testing.T) {
	native, ok := upstreamadapter.Lookup("")
	if !ok {
		t.Fatal("native adapter missing")
	}
	body := []byte(`{"opaque":9007199254740993,"unknown_extension":{"value":1}}`)
	wire, err := native.Prepare(upstreamadapter.Input{Body: body})
	if err != nil || !bytes.Equal(wire.Body, body) {
		t.Fatal("native wire must remain exact")
	}
	if unknown, ok := upstreamadapter.Lookup("unknown-private-marker"); ok {
		t.Fatal("unknown adapter was accepted")
	} else if _, err := unknown.Prepare(upstreamadapter.Input{Body: body}); err == nil || strings.Contains(err.Error(), "private-marker") {
		t.Fatal("invalid definition must fail without echoing private input")
	}
	if upstreamadapter.Templates("unknown-private-marker") != nil {
		t.Fatal("unknown profile received fallback defaults")
	}
}

func TestNanoGPTPreparedToolStateIsRequestScoped(t *testing.T) {
	definition, _ := upstreamadapter.Lookup("nanogpt")
	prepare := func(name string) *upstreamadapter.Wire {
		t.Helper()
		body := []byte(`{"model":"fixture","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"` + name + `","parameters":{"type":"object"}}}]}`)
		wire, err := definition.Prepare(upstreamadapter.Input{Body: body})
		if err != nil {
			t.Fatal(err)
		}
		return wire
	}
	first, second := prepare("first"), prepare("second")
	if !first.NanoGPTTools.Allows("first") || first.NanoGPTTools.Allows("second") || !second.NanoGPTTools.Allows("second") || second.NanoGPTTools.Allows("first") {
		t.Fatal("tool declarations escaped their request")
	}
	if first.NanoGPTTools.IdentitySeed() == second.NanoGPTTools.IdentitySeed() {
		t.Fatal("different requests share a synthesized-tool identity seed")
	}
}
