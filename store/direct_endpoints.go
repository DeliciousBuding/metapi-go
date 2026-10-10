package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// DirectEndpoint is a fully resolved HTTP endpoint. The importer owns source
// provider URL conventions; execution must not normalize this URL again.
type DirectEndpoint struct {
	URL  string `json:"url"`
	Auth string `json:"auth"`
	// ModelPath appends the native Gemini or Bedrock model/action to its prefix.
	// Custom endpoint URLs remain exact when false.
	ModelPath bool   `json:"modelPath,omitempty"`
	Profile   string `json:"profile,omitempty"`
	// RequestModel is the Responses model used by the Codex image bridge;
	// the grant's model remains the image tool model.
	RequestModel string `json:"requestModel,omitempty"`
}

const (
	DirectAuthBearer = "bearer"
	DirectAuthAPIKey = "x-api-key"
	DirectAuthGoogle = "x-goog-api-key"
	DirectAuthNone   = "none"
)

// DirectEndpoints is optional for imports that use the original base/path
// contract. When present, each endpoint owns both its URL and authentication.
type DirectEndpoints struct {
	Chat                      *DirectEndpoint `json:"chat,omitempty"`
	Responses                 *DirectEndpoint `json:"responses,omitempty"`
	Messages                  *DirectEndpoint `json:"messages,omitempty"`
	Gemini                    *DirectEndpoint `json:"gemini,omitempty"`
	Completions               *DirectEndpoint `json:"completions,omitempty"`
	Embeddings                *DirectEndpoint `json:"embeddings,omitempty"`
	Rerank                    *DirectEndpoint `json:"rerank,omitempty"`
	ImageGeneration           *DirectEndpoint `json:"imageGeneration,omitempty"`
	ImageEdit                 *DirectEndpoint `json:"imageEdit,omitempty"`
	ImageVariation            *DirectEndpoint `json:"imageVariation,omitempty"`
	AudioSpeech               *DirectEndpoint `json:"audioSpeech,omitempty"`
	AudioTranscription        *DirectEndpoint `json:"audioTranscription,omitempty"`
	AudioTranslation          *DirectEndpoint `json:"audioTranslation,omitempty"`
	Moderations               *DirectEndpoint `json:"moderations,omitempty"`
	Video                     *DirectEndpoint `json:"video,omitempty"`
	GeminiEmbeddings          *DirectEndpoint `json:"geminiEmbeddings,omitempty"`
	JinaEmbeddings            *DirectEndpoint `json:"jinaEmbeddings,omitempty"`
	ModelScopeImageGeneration *DirectEndpoint `json:"modelscopeImageGeneration,omitempty"`
	SeedanceVideo             *DirectEndpoint `json:"seedanceVideo,omitempty"`
	ZenmuxVideo               *DirectEndpoint `json:"zenmuxVideo,omitempty"`
	Ollama                    *DirectEndpoint `json:"ollama,omitempty"`
	SystemOne                 *DirectEndpoint `json:"systemOne,omitempty"`
	AlphaSearch               *DirectEndpoint `json:"alphaSearch,omitempty"`
}

var requiredDirectProfiles = map[int]string{
	DirectProtocolJinaEmbeddings:            "jina-embeddings",
	DirectProtocolModelScopeImageGeneration: "modelscope-image",
	DirectProtocolSeedanceVideo:             "seedance-video",
	DirectProtocolZenmuxVideo:               "zenmux-video",
	DirectProtocolOllama:                    "ollama",
}

func (e DirectEndpoints) IsConfigured() bool {
	return e.ProtocolMask() != 0
}

// AnonymousProtocolMask is deliberately narrower than the endpoint mask.
// A credential without secret material cannot authorize authenticated slots.
func (e DirectEndpoints) AnonymousProtocolMask() int {
	mask := 0
	for _, entry := range e.Entries() {
		if ep := entry.Endpoint; ep != nil && ep.Auth == DirectAuthNone && (ep.Profile == "ollama" || ep.Profile == "ollama-messages") {
			mask |= entry.Protocol
		}
	}
	return mask
}

func DirectProviderAllowsAnonymous(provider string) bool {
	return provider == "ollama" || provider == "ollama_anthropic"
}

func (e DirectEndpoints) Value() (driver.Value, error) {
	b, err := json.Marshal(e)
	return string(b), err
}

func (e *DirectEndpoints) Scan(value any) error {
	var data []byte
	switch value := value.(type) {
	case string:
		data = []byte(value)
	case []byte:
		data = value
	case nil:
		*e = DirectEndpoints{}
		return nil
	default:
		return fmt.Errorf("invalid direct endpoint configuration type %T", value)
	}
	var decoded DirectEndpoints
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("invalid direct endpoint configuration")
	}
	for _, entry := range decoded.Entries() {
		endpoint := entry.Endpoint
		if endpoint == nil {
			continue
		}
		u, err := url.Parse(endpoint.URL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
			return fmt.Errorf("invalid direct endpoint URL")
		}
		if endpoint.Auth != DirectAuthBearer && endpoint.Auth != DirectAuthAPIKey && endpoint.Auth != DirectAuthGoogle && endpoint.Auth != DirectAuthNone {
			return fmt.Errorf("invalid direct endpoint authentication")
		}
		if endpoint.Auth == DirectAuthNone && endpoint.Profile != "ollama" && endpoint.Profile != "ollama-messages" {
			return fmt.Errorf("unauthenticated endpoints require an Ollama wire profile")
		}
		if endpoint.ModelPath && endpoint != decoded.Gemini && endpoint != decoded.GeminiEmbeddings && !(endpoint == decoded.Messages && endpoint.Profile == "bedrock") {
			return fmt.Errorf("model paths require a Gemini or Bedrock endpoint")
		}
		if endpoint.RequestModel != "" && endpoint.Profile != "codex-image" {
			return fmt.Errorf("request model is only supported by the Codex image profile")
		}
		if required := requiredDirectProfiles[entry.Protocol]; required != "" && endpoint.Profile != required {
			return fmt.Errorf("provider-specific endpoint requires its wire profile")
		}
		switch endpoint.Profile {
		case "":
		case "codex":
			if endpoint != decoded.Responses || endpoint.Auth != DirectAuthBearer {
				return fmt.Errorf("codex profile requires bearer Responses")
			}
		case "claudecode":
			if endpoint != decoded.Messages || endpoint.Auth != DirectAuthBearer {
				return fmt.Errorf("claudecode profile requires bearer Messages")
			}
		case "deepseek", "zai":
			if endpoint != decoded.Chat || endpoint.Auth != DirectAuthBearer {
				return fmt.Errorf("chat profile requires bearer Chat")
			}
		case "ollama", "ollama-messages":
			validSlot := endpoint.Profile == "ollama" && entry.Protocol == DirectProtocolOllama || endpoint.Profile == "ollama-messages" && entry.Protocol == DirectProtocolMessages
			if !validSlot || (endpoint.Auth != DirectAuthBearer && endpoint.Auth != DirectAuthNone) {
				return fmt.Errorf("Ollama profile requires its native endpoint and optional bearer authentication")
			}
		case "bedrock":
			if entry.Protocol != DirectProtocolMessages || endpoint.Auth != DirectAuthBearer || !endpoint.ModelPath {
				return fmt.Errorf("Bedrock requires bearer Messages and a model URL prefix")
			}
		case "seedance-video", "zenmux-video":
			validSlot := endpoint.Profile == "seedance-video" && entry.Protocol == DirectProtocolSeedanceVideo || endpoint.Profile == "zenmux-video" && entry.Protocol == DirectProtocolZenmuxVideo
			if !validSlot || endpoint.Auth != DirectAuthBearer {
				return fmt.Errorf("native video profile requires its matching bearer endpoint")
			}
		case "codex-alpha-search":
			if entry.Protocol != DirectProtocolAlphaSearch || endpoint.Auth != DirectAuthBearer {
				return fmt.Errorf("Codex alpha search requires its bearer endpoint")
			}
		case "jina-embeddings":
			if entry.Protocol != DirectProtocolJinaEmbeddings || endpoint.Auth != DirectAuthBearer {
				return fmt.Errorf("Jina profile requires bearer embeddings")
			}
		case "minimax-image":
			if entry.Protocol != DirectProtocolImageGeneration || endpoint.Auth != DirectAuthBearer {
				return fmt.Errorf("MiniMax image profile requires bearer image generation")
			}
		case "modelscope-image":
			if (entry.Protocol != DirectProtocolImageGeneration && entry.Protocol != DirectProtocolImageEdit && entry.Protocol != DirectProtocolModelScopeImageGeneration) || endpoint.Auth != DirectAuthBearer {
				return fmt.Errorf("ModelScope image profile requires bearer image generation or editing")
			}
		case "codex-image":
			if (entry.Protocol != DirectProtocolImageGeneration && entry.Protocol != DirectProtocolImageEdit) || endpoint.Auth != DirectAuthBearer || strings.TrimSpace(endpoint.RequestModel) == "" || len(endpoint.RequestModel) > 255 {
				return fmt.Errorf("Codex image profile requires bearer image generation or editing and a request model")
			}
		default:
			return fmt.Errorf("invalid direct endpoint profile")
		}
	}
	*e = decoded
	return nil
}

// DirectProtocolOrder restricts one route item without broadening its shared grant.
type DirectProtocolOrder []int

func (p DirectProtocolOrder) Value() (driver.Value, error) {
	if p == nil {
		return "[]", nil
	}
	b, err := json.Marshal(p)
	return string(b), err
}
func (p *DirectProtocolOrder) Scan(value any) error {
	var raw []byte
	switch v := value.(type) {
	case nil:
		*p = nil
		return nil
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("invalid direct protocol order")
	}
	var values []int
	if err := json.Unmarshal(raw, &values); err != nil {
		return fmt.Errorf("invalid direct protocol order")
	}
	seen := map[int]bool{}
	for _, v := range values {
		if !ValidDirectProtocol(v) || seen[v] {
			return fmt.Errorf("invalid direct protocol order")
		}
		seen[v] = true
	}
	*p = values
	return nil
}
