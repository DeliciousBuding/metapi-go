// Package upstreamadapter owns direct-provider wire differences and the
// endpoint templates that select them. It performs no network I/O, credential
// lookup or route selection; existing transforms and transport owners are reused.
package upstreamadapter

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
	"github.com/deliciousbuding/metapi-go/transform/ollama"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
)

// Template is a relative provider default, not a protocol permission. Product
// catalogs own base URLs and may combine templates with native endpoints.
type Template struct {
	Protocol     string
	Path         string
	Auth         string
	Profile      string
	ModelPath    bool
	RequestModel string
}

// Input contains only the selected wire and already-resolved identity metadata.
// The caller keeps the secret, credential lifecycle and authenticated key.
type Input struct {
	Body             []byte
	Downstream       http.Header
	EndpointURL      string
	ChannelID        int64
	OAuth            bool
	AccountID        string
	SessionNamespace string
}

// Wire is request-scoped. Body preparation happens after the caller's protocol
// conversion and parameter overrides; headers are applied after generic auth.
type Wire struct {
	Body               []byte
	Headers            http.Header
	Query              url.Values
	RemoveHeaders      []string
	RemoveEmptyHeaders []string
	ForceStream        bool
	StripToolPrefix    bool
	NanoGPTTools       *NanoGPTTools
}

type prepareFunc func(Input, *Wire) error
type projectFunc func([]byte, bool) ([]byte, error)

// Definition connects executable preparation/projection to its exact defaults.
// Fields are private so lookups cannot mutate shared callbacks or templates.
type Definition struct {
	templates []Template
	prepare   prepareFunc
	project   projectFunc
	restore   func([]byte, bool) []byte
}

func (d Definition) Prepare(in Input) (*Wire, error) {
	if d.prepare == nil {
		return nil, fmt.Errorf("unsupported direct provider profile")
	}
	wire := &Wire{Body: in.Body, Headers: make(http.Header), Query: make(url.Values)}
	if err := d.prepare(in, wire); err != nil {
		return nil, err
	}
	return wire, nil
}

// ProjectResponse runs only before a downstream protocol bridge. Native JSON
// and SSE retain provider extensions under their existing response normalizer.
func (d Definition) ProjectResponse(body []byte, stream bool) ([]byte, error) {
	if d.project == nil {
		return nil, fmt.Errorf("unsupported direct provider profile")
	}
	return d.project(body, stream)
}

// RestoreResponse reverses request-scoped tool naming before both native output
// and cross-protocol projection. The caller owns SSE parsing and terminal checks.
func (d Definition) RestoreResponse(body []byte, stripToolPrefix bool) []byte {
	if d.restore != nil {
		return d.restore(body, stripToolPrefix)
	}
	return body
}

func Lookup(profile string) (Definition, bool) {
	definition, ok := definitions[profile]
	return definition, ok
}

// Templates returns fresh values; resolving one connection must not change
// future defaults. Native passthrough and unknown profiles have no templates.
func Templates(profile string) []Template {
	return slices.Clone(definitions[profile].templates)
}

// Profiles includes the empty native-passthrough profile in sorted order.
func Profiles() []string {
	profiles := make([]string, 0, len(definitions))
	for profile := range definitions {
		profiles = append(profiles, profile)
	}
	slices.Sort(profiles)
	return profiles
}

func passthroughPrepare(Input, *Wire) error { return nil }
func passthroughProject(body []byte, _ bool) ([]byte, error) {
	return body, nil
}

func bodyPrepare(prepare func([]byte) ([]byte, error)) prepareFunc {
	return func(in Input, wire *Wire) error {
		var err error
		wire.Body, err = prepare(in.Body)
		return err
	}
}

func profileDefinition(profile, protocol, path, auth string, prepare prepareFunc) Definition {
	return Definition{
		templates: []Template{{Protocol: protocol, Path: path, Auth: auth, Profile: profile}},
		prepare:   prepare,
		project:   passthroughProject,
	}
}

func chatDefinition(profile string, prepare prepareFunc, project projectFunc) Definition {
	definition := profileDefinition(profile, "chat", "/v1/chat/completions", "bearer", prepare)
	if project != nil {
		definition.project = project
	}
	return definition
}

var definitions = map[string]Definition{
	"": {prepare: passthroughPrepare, project: passthroughProject},
	"moonshot": chatDefinition("moonshot", bodyPrepare(func(body []byte) ([]byte, error) {
		return prepareDirectDomesticProfileRequest(body, "moonshot")
	}), nil),
	"longcat": chatDefinition("longcat", bodyPrepare(func(body []byte) ([]byte, error) {
		return prepareDirectDomesticProfileRequest(body, "longcat")
	}), nil),
	"openrouter": chatDefinition("openrouter", bodyPrepare(func(body []byte) ([]byte, error) {
		return prepareDirectRouterChatRequest(body, "openrouter")
	}), projectDirectRouterChatResponse),
	"cerebras": chatDefinition("cerebras", bodyPrepare(func(body []byte) ([]byte, error) {
		return prepareDirectRouterChatRequest(body, "cerebras")
	}), projectDirectRouterChatResponse),
	"nanogpt": chatDefinition("nanogpt", func(in Input, wire *Wire) error {
		var err error
		wire.NanoGPTTools, err = NewNanoGPTTools(in.Body)
		if err == nil {
			wire.Body, err = prepareDirectNanoGPTRequest(in.Body)
		}
		return err
	}, nil),
	"bailian": chatDefinition("bailian", bodyPrepare(prepareDirectBailianRequest), nil),
	"cline": chatDefinition("cline", func(in Input, wire *Wire) error {
		var err error
		wire.Body, err = prepareDirectClineRequest(in.Body)
		if err == nil {
			wire.Headers.Set("X-Client-Type", "cline-cli")
		}
		return err
	}, projectDirectClineReasoning),
	"github-copilot": profileDefinition("github-copilot", "chat", "/chat/completions", "bearer", func(in Input, wire *Wire) error {
		buildDirectCopilotHeaders(wire.Headers, in.Downstream)
		return nil
	}),
	"xai-subscription": profileDefinition("xai-subscription", "responses", "/v1/responses", "bearer", func(_ Input, wire *Wire) error {
		buildDirectXAISubscriptionHeaders(wire.Headers)
		wire.RemoveHeaders = []string{"X-Xai-Token-Auth", "X-Grok-Client-Version", "X-Grok-Client-Identifier", "User-Agent"}
		return nil
	}),
	"ollama":          profileDefinition("ollama", "ollama", "/api/chat", "none", bodyPrepare(ollama.FromChatRequest)),
	"ollama-messages": profileDefinition("ollama-messages", "messages", "/v1/messages", "none", passthroughPrepare),
	"bedrock": {
		templates: []Template{{Protocol: "messages", Path: "/model", Auth: "bearer", Profile: "bedrock", ModelPath: true}},
		prepare: func(in Input, wire *Wire) error {
			var err error
			wire.Body, err = messages.PrepareBedrockRequest(in.Body, in.Downstream.Values("Anthropic-Beta"))
			if err != nil {
				return err
			}
			wire.Headers.Set("Anthropic-Version", "bedrock-2023-05-31")
			wire.Headers.Set("Accept", "application/json")
			wire.Headers["Anthropic-Beta"] = nil
			return nil
		},
		project: passthroughProject,
	},
	// Vertex's bearer credential and model/action URL are handled by the caller.
	// The Gemini body is already native, including tools and integer lexemes.
	"vertex": {
		templates: []Template{
			{Protocol: "gemini", Path: "/v1/projects/{project}/locations/{region}/publishers/google/models", Auth: "bearer", Profile: "vertex", ModelPath: true},
			{Protocol: "geminiEmbeddings", Path: "/v1/projects/{project}/locations/{region}/publishers/google/models", Auth: "bearer", Profile: "vertex", ModelPath: true},
		},
		prepare: passthroughPrepare,
		project: passthroughProject,
	},
	"vertex-anthropic": {
		templates: []Template{{Protocol: "messages", Path: "/v1/projects/{project}/locations/{region}/publishers/anthropic/models", Auth: "bearer", Profile: "vertex-anthropic", ModelPath: true}},
		prepare: func(in Input, wire *Wire) error {
			var err error
			wire.Body, err = messages.PrepareVertexRequest(in.Body)
			if err == nil {
				wire.Headers["Anthropic-Version"] = nil
			}
			return err
		},
		project: passthroughProject,
	},
	"codex-alpha-search": profileDefinition("codex-alpha-search", "alphaSearch", "/alpha/search", "bearer", func(in Input, wire *Wire) error {
		prepareCodexHeaders(in, wire)
		wire.Headers.Set("Accept", "application/json")
		return nil
	}),
	"jina-embeddings": profileDefinition("jina-embeddings", "jinaEmbeddings", "/v1/embeddings", "bearer", bodyPrepare(prepareJinaEmbeddings)),
	"deepseek": chatDefinition("deepseek", bodyPrepare(func(body []byte) ([]byte, error) {
		return PrepareDomesticChatRequest(body, "deepseek")
	}), nil),
	"zai": chatDefinition("zai", bodyPrepare(func(body []byte) ([]byte, error) {
		return PrepareDomesticChatRequest(body, "zai")
	}), nil),
	"codex": profileDefinition("codex", "responses", "/responses", "bearer", prepareCodex),
	"codex-image": {
		templates: []Template{
			{Protocol: "imageGeneration", Path: "/responses", Auth: "bearer", Profile: "codex-image", RequestModel: "gpt-6-luna"},
			{Protocol: "imageEdit", Path: "/responses", Auth: "bearer", Profile: "codex-image", RequestModel: "gpt-6-luna"},
		},
		prepare: prepareCodex,
		project: passthroughProject,
	},
	"claudecode": {
		templates: []Template{{Protocol: "messages", Path: "/v1/messages", Auth: "bearer", Profile: "claudecode"}},
		prepare: func(in Input, wire *Wire) error {
			cli := strings.HasPrefix(in.Downstream.Get("User-Agent"), "claude-cli/")
			wire.StripToolPrefix = in.OAuth && !cli
			userID := directClaudeUserID(in.Body, in.ChannelID, in.Downstream, in.SessionNamespace)
			var err error
			wire.Body, err = messages.PrepareClaudeCodeRequest(in.Body, userID, wire.StripToolPrefix)
			if err != nil {
				return err
			}
			buildDirectClaudeHeaders(wire.Headers, in.Downstream, cli, in.SessionNamespace)
			wire.Query.Set("beta", "true")
			return nil
		},
		project: passthroughProject,
		restore: messages.RestoreClaudeCodeTools,
	},
}

func prepareCodex(in Input, wire *Wire) error {
	var err error
	wire.Body, err = responses.PrepareCodexRequest(in.Body)
	if err != nil {
		return err
	}
	wire.ForceStream = true
	prepareCodexHeaders(in, wire)
	return nil
}

func prepareCodexHeaders(in Input, wire *Wire) {
	buildDirectCodexHeaders(wire.Headers, in.Downstream, in.AccountID, in.EndpointURL, in.SessionNamespace)
	wire.RemoveHeaders = []string{"X-Openai-Internal-Codex-Responses-Lite", "Session_id"}
	wire.RemoveEmptyHeaders = []string{"Chatgpt-Account-Id"}
}
