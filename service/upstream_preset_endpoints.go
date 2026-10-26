package service

import (
	"github.com/deliciousbuding/metapi-go/internal/upstreamadapter"
	"github.com/deliciousbuding/metapi-go/store"
)

// UpstreamPresetEndpointPaths returns every executable endpoint of a product.
// Paths are relative to the per-protocol bases on the resolved preset. The
// platform parameter remains for source compatibility with older callers.
func UpstreamPresetEndpointPaths(id, platform string) store.DirectEndpoints {
	if preset := GetUpstreamPreset(id); preset != nil {
		return preset.endpoints
	}
	return store.DirectEndpoints{}
}

// The same adapter definition supplies defaults here and prepares the real
// request in handler/proxy. Product membership and per-protocol hosts remain
// in the preset catalog; templates never limit explicit endpointConfig.
func upstreamAdapterEndpoint(profile, protocol string) *store.DirectEndpoint {
	for _, template := range upstreamadapter.Templates(profile) {
		if template.Protocol == protocol {
			return &store.DirectEndpoint{
				URL: template.Path, Auth: template.Auth, Profile: template.Profile,
				ModelPath: template.ModelPath, RequestModel: template.RequestModel,
			}
		}
	}
	return nil
}

func upstreamPresetVariantEndpoints(id, platform string) store.DirectEndpoints {
	endpoint := func(path string) *store.DirectEndpoint {
		return &store.DirectEndpoint{URL: path, Auth: store.DirectAuthBearer}
	}
	gemini := func() *store.DirectEndpoint {
		return &store.DirectEndpoint{URL: "/v1beta/models", Auth: store.DirectAuthGoogle, ModelPath: true}
	}
	compatible := func(config *store.DirectEndpoints, audio bool) {
		config.Embeddings = endpoint("/v1/embeddings")
		config.ImageGeneration = endpoint("/v1/images/generations")
		config.ImageEdit = endpoint("/v1/images/edits")
		config.ImageVariation = endpoint("/v1/images/variations")
		config.Moderations = endpoint("/v1/moderations")
		config.Video = endpoint("/v1/videos")
		if audio {
			config.AudioSpeech = endpoint("/v1/audio/speech")
			config.AudioTranscription = endpoint("/v1/audio/transcriptions")
			config.AudioTranslation = endpoint("/v1/audio/translations")
		}
	}
	var config store.DirectEndpoints
	switch platform {
	case "custom", "sub2api":
		config.Chat = endpoint("/v1/chat/completions")
		config.Responses = endpoint("/v1/responses")
		config.Messages = endpoint("/v1/messages")
		config.Gemini = gemini()
		if platform == "custom" {
			config.Messages.Auth = store.DirectAuthAPIKey
		} else {
			config.Gemini.Auth = store.DirectAuthBearer
		}
	case "vllm", "sglang":
		// New API's native server adapters send all five wire formats without
		// conversion, including Bearer-authenticated Anthropic Messages.
		config.Chat = endpoint("/v1/chat/completions")
		config.Responses = endpoint("/v1/responses")
		config.Messages = endpoint("/v1/messages")
		config.Completions = endpoint("/v1/completions")
		config.Embeddings = endpoint("/v1/embeddings")
	case "new-api":
		config.Chat = endpoint("/v1/chat/completions")
		config.Responses = endpoint("/v1/responses")
		config.Messages = endpoint("/v1/messages")
		config.Gemini = gemini()
		config.Completions = endpoint("/v1/completions")
		config.Embeddings = endpoint("/v1/embeddings")
		config.Rerank = endpoint("/v1/rerank")
		config.ImageGeneration = endpoint("/v1/images/generations")
		config.ImageEdit = endpoint("/v1/images/edits")
		config.AudioSpeech = endpoint("/v1/audio/speech")
		config.AudioTranscription = endpoint("/v1/audio/transcriptions")
		config.AudioTranslation = endpoint("/v1/audio/translations")
		config.Moderations = endpoint("/v1/moderations")
		config.Video = endpoint("/v1/videos")
		// New API explicitly returns RelayNotImplemented for image variations.
	case "openai":
		config.Chat = endpoint("/v1/chat/completions")
	case "claude":
		config.Messages = &store.DirectEndpoint{URL: "/v1/messages", Auth: store.DirectAuthAPIKey}
	case "gemini":
		config.Gemini = gemini()
		config.GeminiEmbeddings = gemini()
	}
	// Match the particular product rather than its vendor: Coding Plan and
	// Anthropic-compatible products do not inherit a vendor's media endpoints.
	switch id {
	case "opencode-go":
		config = store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: "/v1/chat/completions", Auth: store.DirectAuthBearer, Profile: "opencode-go", ModelWireURLs: &store.DirectModelWireURLs{Responses: "/v1/responses", Messages: "/v1/messages"}}}
		config.Responses = endpoint("/v1/responses")
	case "opencode-go-messages":
		config = store.DirectEndpoints{Messages: &store.DirectEndpoint{URL: "/v1/messages", Auth: store.DirectAuthAPIKey}}
	case "cline":
		config = store.DirectEndpoints{Chat: upstreamAdapterEndpoint("cline", "chat")}
	case "codingplan-openai":
		config.Chat = upstreamAdapterEndpoint("bailian", "chat")
	case "zhipu-openai", "zai-openai", "zhipu-coding-plan-openai", "zai-coding-plan-openai", "xiaomi-openai":
		config.Chat = upstreamAdapterEndpoint("zai", "chat")
	case "moonshot-openai", "kimi-coding-openai":
		config.Chat = upstreamAdapterEndpoint("moonshot", "chat")
	case "longcat":
		config.Chat = upstreamAdapterEndpoint("longcat", "chat")
	case "longcat-messages":
		config.Messages.Auth = store.DirectAuthBearer
	case "nanogpt", "zenmux":
		compatible(&config, true)
		if id == "nanogpt" {
			config.Chat = upstreamAdapterEndpoint("nanogpt", "chat")
		}
	case "nanogpt-responses", "zenmux-responses":
		config = store.DirectEndpoints{Responses: endpoint("/v1/responses")}
	case "ppio-openai", "siliconflow":
		compatible(&config, false)
	case "cerebras":
		config.Chat = upstreamAdapterEndpoint("cerebras", "chat")
	case "ollama-native":
		config = store.DirectEndpoints{Ollama: upstreamAdapterEndpoint("ollama", "ollama")}
	case "ollama-anthropic":
		config = store.DirectEndpoints{Messages: upstreamAdapterEndpoint("ollama-messages", "messages")}
	case "bedrock-messages":
		config = store.DirectEndpoints{Messages: upstreamAdapterEndpoint("bedrock", "messages")}
	case "vertex-gemini":
		config = store.DirectEndpoints{Gemini: upstreamAdapterEndpoint("vertex", "gemini"), GeminiEmbeddings: upstreamAdapterEndpoint("vertex", "geminiEmbeddings")}
	case "vertex-anthropic":
		config = store.DirectEndpoints{Messages: upstreamAdapterEndpoint("vertex-anthropic", "messages")}
	case "seedance-video":
		config = store.DirectEndpoints{SeedanceVideo: &store.DirectEndpoint{URL: "/contents/generations/tasks", Auth: store.DirectAuthBearer, Profile: "seedance-video"}}
	case "zenmux-video":
		config = store.DirectEndpoints{ZenmuxVideo: &store.DirectEndpoint{URL: "/videos", Auth: store.DirectAuthBearer, Profile: "zenmux-video"}}
	case "typesafe-systemone":
		config = store.DirectEndpoints{SystemOne: endpoint("/systemone")}
	case "openai-alpha-search":
		config = store.DirectEndpoints{AlphaSearch: endpoint("/alpha/search")}
	case "codex-alpha-search":
		config = store.DirectEndpoints{AlphaSearch: upstreamAdapterEndpoint("codex-alpha-search", "alphaSearch")}
	case "codex-responses":
		config = store.DirectEndpoints{Responses: upstreamAdapterEndpoint("codex", "responses")}
	case "codex-images":
		config = store.DirectEndpoints{
			ImageGeneration: upstreamAdapterEndpoint("codex-image", "imageGeneration"),
			ImageEdit:       upstreamAdapterEndpoint("codex-image", "imageEdit"),
		}
	case "claude-code":
		config = store.DirectEndpoints{Messages: upstreamAdapterEndpoint("claudecode", "messages")}
	case "openai-api":
		compatible(&config, true)
		config.Responses = endpoint("/v1/responses")
	case "bailian":
		config.Chat = upstreamAdapterEndpoint("bailian", "chat")
		config.Responses = endpoint("/v1/responses")
	case "xai-api":
		config.Responses = endpoint("/v1/responses")
	case "deepseek-openai":
		config.Chat = upstreamAdapterEndpoint("deepseek", "chat")
		config.Completions = endpoint("/beta/completions")
	case "minimax-openai":
		config.ImageGeneration = endpoint("/v1/image_generation")
		config.ImageGeneration.Profile = "minimax-image"
	case "modelscope-openai":
		config.ModelScopeImageGeneration = endpoint("/v1/images/generations")
		config.ModelScopeImageGeneration.Profile = "modelscope-image"
	case "jina":
		config.Chat = nil
		config.JinaEmbeddings = upstreamAdapterEndpoint("jina-embeddings", "jinaEmbeddings")
		config.Rerank = endpoint("/v1/rerank")
	case "groq", "openrouter":
		config.AudioSpeech = endpoint("/v1/audio/speech")
		config.AudioTranscription = endpoint("/v1/audio/transcriptions")
		config.AudioTranslation = endpoint("/v1/audio/translations")
		if id == "openrouter" {
			config.Chat = upstreamAdapterEndpoint("openrouter", "chat")
			config.ImageGeneration = &store.DirectEndpoint{URL: "/v1/images", Auth: store.DirectAuthBearer, Profile: "openrouter-image"}
			config.ImageEdit = &store.DirectEndpoint{URL: "/v1/images", Auth: store.DirectAuthBearer, Profile: "openrouter-image"}
		}
	}
	return config
}
