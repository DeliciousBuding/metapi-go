package service

import "github.com/deliciousbuding/metapi-go/store"

// UpstreamPresetEndpointPaths owns the executable defaults of native presets.
// URLs here are relative paths; the admin resolver joins them to the chosen
// base URL before validation or persistence. Listing capabilities uses this
// same configuration, so the gallery cannot advertise an unconfigured endpoint.
func UpstreamPresetEndpointPaths(id, platform string) store.DirectEndpoints {
	endpoint := func(path string) *store.DirectEndpoint {
		return &store.DirectEndpoint{URL: path, Auth: store.DirectAuthBearer}
	}
	gemini := func() *store.DirectEndpoint {
		return &store.DirectEndpoint{URL: "/v1beta/models", Auth: store.DirectAuthGoogle, ModelPath: true}
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
	case "openai-api":
		config.Responses = endpoint("/v1/responses")
		config.Embeddings = endpoint("/v1/embeddings")
		config.ImageGeneration = endpoint("/v1/images/generations")
		config.ImageEdit = endpoint("/v1/images/edits")
		config.ImageVariation = endpoint("/v1/images/variations")
		config.AudioSpeech = endpoint("/v1/audio/speech")
		config.AudioTranscription = endpoint("/v1/audio/transcriptions")
		config.AudioTranslation = endpoint("/v1/audio/translations")
		config.Moderations = endpoint("/v1/moderations")
	case "xai-api", "bailian":
		config.Responses = endpoint("/v1/responses")
	case "deepseek-openai":
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
	}
	return config
}
