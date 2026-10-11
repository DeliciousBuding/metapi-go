package service

import "github.com/deliciousbuding/metapi-go/store"

// UpstreamPresetEndpointPaths returns every executable endpoint of a product.
// Paths are relative to the per-protocol bases on the resolved preset. The
// platform parameter remains for source compatibility with older callers.
func UpstreamPresetEndpointPaths(id, platform string) store.DirectEndpoints {
	if preset := GetUpstreamPreset(id); preset != nil {
		return preset.endpoints
	}
	return store.DirectEndpoints{}
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
		config = store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: "/v1/chat/completions", Auth: store.DirectAuthBearer, Profile: "cline"}}
	case "codingplan-openai":
		config.Chat.Profile = "bailian"
	case "zhipu-openai", "zai-openai", "zhipu-coding-plan-openai", "zai-coding-plan-openai", "xiaomi-openai":
		config.Chat.Profile = "zai"
	case "moonshot-openai", "kimi-coding-openai":
		config.Chat.Profile = "moonshot"
	case "longcat":
		config.Chat.Profile = "longcat"
	case "longcat-messages":
		config.Messages.Auth = store.DirectAuthBearer
	case "nanogpt", "zenmux":
		compatible(&config, true)
		if id == "nanogpt" {
			config.Chat.Profile = "nanogpt"
		}
	case "nanogpt-responses", "zenmux-responses":
		config = store.DirectEndpoints{Responses: endpoint("/v1/responses")}
	case "ppio-openai", "siliconflow":
		compatible(&config, false)
	case "cerebras":
		config.Chat.Profile = "cerebras"
	case "ollama-native":
		config = store.DirectEndpoints{Ollama: &store.DirectEndpoint{URL: "/api/chat", Auth: store.DirectAuthNone, Profile: "ollama"}}
	case "ollama-anthropic":
		config = store.DirectEndpoints{Messages: &store.DirectEndpoint{URL: "/v1/messages", Auth: store.DirectAuthNone, Profile: "ollama-messages"}}
	case "bedrock-messages":
		config = store.DirectEndpoints{Messages: &store.DirectEndpoint{URL: "/model", Auth: store.DirectAuthBearer, Profile: "bedrock", ModelPath: true}}
	case "seedance-video":
		config = store.DirectEndpoints{SeedanceVideo: &store.DirectEndpoint{URL: "/contents/generations/tasks", Auth: store.DirectAuthBearer, Profile: "seedance-video"}}
	case "zenmux-video":
		config = store.DirectEndpoints{ZenmuxVideo: &store.DirectEndpoint{URL: "/videos", Auth: store.DirectAuthBearer, Profile: "zenmux-video"}}
	case "typesafe-systemone":
		config = store.DirectEndpoints{SystemOne: endpoint("/systemone")}
	case "openai-alpha-search":
		config = store.DirectEndpoints{AlphaSearch: endpoint("/alpha/search")}
	case "codex-alpha-search":
		config = store.DirectEndpoints{AlphaSearch: &store.DirectEndpoint{URL: "/alpha/search", Auth: store.DirectAuthBearer, Profile: "codex-alpha-search"}}
	case "codex-responses":
		config = store.DirectEndpoints{Responses: &store.DirectEndpoint{URL: "/responses", Auth: store.DirectAuthBearer, Profile: "codex"}}
	case "codex-images":
		config = store.DirectEndpoints{
			ImageGeneration: &store.DirectEndpoint{URL: "/responses", Auth: store.DirectAuthBearer, Profile: "codex-image", RequestModel: "gpt-6-luna"},
			ImageEdit:       &store.DirectEndpoint{URL: "/responses", Auth: store.DirectAuthBearer, Profile: "codex-image", RequestModel: "gpt-6-luna"},
		}
	case "claude-code":
		config = store.DirectEndpoints{Messages: &store.DirectEndpoint{URL: "/v1/messages", Auth: store.DirectAuthBearer, Profile: "claudecode"}}
	case "openai-api":
		compatible(&config, true)
		config.Responses = endpoint("/v1/responses")
	case "bailian":
		config.Chat.Profile = "bailian"
		config.Responses = endpoint("/v1/responses")
	case "xai-api":
		config.Responses = endpoint("/v1/responses")
	case "deepseek-openai":
		config.Chat.Profile = "deepseek"
		config.Completions = endpoint("/beta/completions")
	case "minimax-openai":
		config.ImageGeneration = endpoint("/v1/image_generation")
		config.ImageGeneration.Profile = "minimax-image"
	case "modelscope-openai":
		config.ModelScopeImageGeneration = endpoint("/v1/images/generations")
		config.ModelScopeImageGeneration.Profile = "modelscope-image"
	case "jina":
		config.Chat = nil
		config.JinaEmbeddings = endpoint("/v1/embeddings")
		config.JinaEmbeddings.Profile = "jina-embeddings"
		config.Rerank = endpoint("/v1/rerank")
	case "groq", "openrouter":
		config.AudioSpeech = endpoint("/v1/audio/speech")
		config.AudioTranscription = endpoint("/v1/audio/transcriptions")
		config.AudioTranslation = endpoint("/v1/audio/translations")
		if id == "openrouter" {
			config.Chat.Profile = "openrouter"
			config.ImageGeneration = &store.DirectEndpoint{URL: "/v1/images", Auth: store.DirectAuthBearer, Profile: "openrouter-image"}
			config.ImageEdit = &store.DirectEndpoint{URL: "/v1/images", Auth: store.DirectAuthBearer, Profile: "openrouter-image"}
		}
	}
	return config
}
