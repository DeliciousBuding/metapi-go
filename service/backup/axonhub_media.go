package backup

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// Media URLs use their actual source outbound builders, not the conversation
// builder's raw-URL convention. In particular, generic OpenAI media still
// appends its path after ##, and video/Gemini embedding ignore EndpointPath.
func resolveAxonHubMediaEndpoint(ch AxonHubSourceChannel, protocol int, base, path string, custom bool) (*store.DirectEndpoint, string) {
	endpoint := &store.DirectEndpoint{Auth: store.DirectAuthBearer}
	if protocol == protoGeminiEmbeddings {
		if strings.Contains(base, "#") {
			return nil, "endpoint_url_mode_unsupported"
		}
		version := "v1beta"
		if strings.HasSuffix(base, "/v1beta") {
			base = strings.TrimSuffix(base, "/v1beta")
		} else if strings.HasSuffix(base, "/v1") {
			base = strings.TrimSuffix(base, "/v1")
			version = "v1"
		}
		endpoint.URL = base + "/" + version + "/models"
		endpoint.Auth = store.DirectAuthGoogle
		endpoint.ModelPath = true
		return endpoint, ""
	}
	if protocol == protoModelScopeImage {
		// ModelScope's dedicated constructor keeps the source base unchanged.
		if strings.Contains(base, "#") {
			return nil, "endpoint_url_mode_unsupported"
		}
		endpoint.URL = strings.TrimRight(base, "/") + "/images/generations"
		endpoint.Profile = "modelscope-image"
		return endpoint, ""
	}
	if (ch.Type == "codex" || ch.Type == "fenno") && (protocol == protoImageGeneration || protocol == protoImageEdit) {
		if base == "https://api.openai.com/v1" {
			base = "https://chatgpt.com/backend-api/codex#"
		}
		resolved, problem := resolveAxonHubEndpointURL(ch.Type, protoResponses, base, "", custom)
		if problem != "" {
			return nil, problem
		}
		endpoint.URL = resolved
		endpoint.Profile = "codex-image"
		model := strings.TrimSpace(ch.DefaultTestModel)
		key := model
		if ch.Settings.LowercaseModelID {
			key = strings.ToLower(key)
		}
		if actual, ok := axonHubChannelEntries(ch)[key]; ok {
			model = actual
		}
		if model == "" || strings.HasPrefix(strings.ToLower(model), "gpt-image-") {
			model = "gpt-6-luna"
		}
		endpoint.RequestModel = model
		return endpoint, ""
	}
	if ch.Type == "minimax" && protocol == protoImageGeneration {
		if strings.HasSuffix(base, "##") {
			return nil, "endpoint_url_mode_unsupported"
		}
		base = normalizeAxonHubBaseURL(base, "v1")
		if path == "" {
			path = "/image_generation"
		}
		if strings.HasPrefix(path, "/v1/") && strings.HasSuffix(base, "/v1") {
			base = strings.TrimSuffix(base, "/v1")
		}
		endpoint.URL = base + path
		endpoint.Profile = "minimax-image"
		return endpoint, ""
	}
	version := "v1"
	if path != "" {
		version = ""
	}
	if ch.Type == "deepseek" && protocol == protoCompletions && !custom {
		if strings.HasSuffix(base, "##") {
			return nil, "endpoint_url_mode_unsupported"
		}
		endpoint.URL = strings.TrimSuffix(normalizeAxonHubBaseURL(base, "v1"), "/v1") + "/beta/completions"
		return endpoint, ""
	}
	if protocol == protoJinaEmbeddings || protocol == protoRerank {
		if strings.HasSuffix(base, "##") {
			return nil, "endpoint_url_mode_unsupported"
		}
		base = normalizeAxonHubBaseURL(base, version)
		if protocol == protoJinaEmbeddings {
			endpoint.Profile = "jina-embeddings"
		}
	} else if strings.HasSuffix(base, "##") {
		base = strings.TrimSuffix(base, "##")
		if protocol == protoCompletions {
			endpoint.URL = base
			return endpoint, ""
		}
	} else {
		base = normalizeAxonHubBaseURL(base, version)
	}
	if protocol == protoVideo {
		path = "/videos"
	}
	if path == "" {
		switch protocol {
		case protoCompletions:
			path = "/completions"
		case protoEmbeddings, protoJinaEmbeddings:
			path = "/embeddings"
		case protoRerank:
			path = "/rerank"
		case protoImageGeneration:
			path = "/images/generations"
		case protoImageEdit:
			path = "/images/edits"
		case protoImageVariation:
			path = "/images/variations"
		case protoAudioSpeech:
			path = "/audio/speech"
		case protoAudioTranscription:
			path = "/audio/transcriptions"
		case protoAudioTranslation:
			path = "/audio/translations"
		case protoModerations:
			path = "/moderations"
		default:
			return nil, "api_format_unsupported"
		}
	}
	endpoint.URL = base + path
	return endpoint, ""
}

func axonHubUsesCodexImage(ch AxonHubSourceChannel) bool {
	if ch.Type != "codex" && ch.Type != "fenno" {
		return false
	}
	for _, ep := range axonHubMergedEndpoints(ch, axonHubProviderTypes[ch.Type]) {
		if ep.APIFormat == "openai/image_generation" || ep.APIFormat == "openai/image_edit" {
			return true
		}
	}
	return false
}

// Model type is a structural visibility hint, not an authorization source.
// Retain the explicit endpoint order; do not manufacture a media grant from a
// chat-only channel just because a Model record calls itself an embedding.
func axonHubMediaModelHasEndpoint(kind string, order store.DirectProtocolOrder) bool {
	var capable int
	switch kind {
	case "embedding":
		capable = protoEmbeddings | protoJinaEmbeddings | protoGeminiEmbeddings
	case "rerank":
		capable = protoRerank
	case "image_generation":
		capable = protoImageGeneration | protoImageEdit | protoImageVariation | protoModelScopeImage
	case "video_generation":
		capable = protoVideo | protoSeedanceVideo | protoZenmuxVideo
	default:
		return true
	}
	for _, bit := range order {
		if bit&capable != 0 {
			return true
		}
	}
	return false
}
