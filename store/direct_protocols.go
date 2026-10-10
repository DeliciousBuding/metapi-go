package store

// These persisted bits are shared by endpoint configuration, grants and route
// member restrictions. Existing bits are never reassigned.
const (
	DirectProtocolChat                      = 1 << 1
	DirectProtocolResponses                 = 1 << 2
	DirectProtocolMessages                  = 1 << 3
	DirectProtocolGemini                    = 1 << 4
	DirectProtocolCompletions               = 1 << 5
	DirectProtocolEmbeddings                = 1 << 6
	DirectProtocolRerank                    = 1 << 7
	DirectProtocolImageGeneration           = 1 << 8
	DirectProtocolImageEdit                 = 1 << 9
	DirectProtocolImageVariation            = 1 << 10
	DirectProtocolAudioSpeech               = 1 << 11
	DirectProtocolAudioTranscription        = 1 << 12
	DirectProtocolAudioTranslation          = 1 << 13
	DirectProtocolModerations               = 1 << 14
	DirectProtocolVideo                     = 1 << 15
	DirectProtocolGeminiEmbeddings          = 1 << 16
	DirectProtocolJinaEmbeddings            = 1 << 17
	DirectProtocolModelScopeImageGeneration = 1 << 18
	DirectGenerationProtocols               = DirectProtocolChat | DirectProtocolResponses | DirectProtocolMessages | DirectProtocolGemini
	DirectAllProtocols                      = (1 << 19) - 2
)

func ValidDirectProtocol(bit int) bool {
	return bit > 0 && bit&(bit-1) == 0 && bit&DirectAllProtocols != 0
}

type DirectEndpointEntry struct {
	Key      string
	Protocol int
	Endpoint *DirectEndpoint
}

// Entries is the single field-to-protocol mapping for the persisted endpoint
// object. Nil entries remain visible to callers building a configuration form.
func (e DirectEndpoints) Entries() []DirectEndpointEntry {
	return []DirectEndpointEntry{
		{"chat", DirectProtocolChat, e.Chat},
		{"responses", DirectProtocolResponses, e.Responses},
		{"messages", DirectProtocolMessages, e.Messages},
		{"gemini", DirectProtocolGemini, e.Gemini},
		{"completions", DirectProtocolCompletions, e.Completions},
		{"embeddings", DirectProtocolEmbeddings, e.Embeddings},
		{"rerank", DirectProtocolRerank, e.Rerank},
		{"imageGeneration", DirectProtocolImageGeneration, e.ImageGeneration},
		{"imageEdit", DirectProtocolImageEdit, e.ImageEdit},
		{"imageVariation", DirectProtocolImageVariation, e.ImageVariation},
		{"audioSpeech", DirectProtocolAudioSpeech, e.AudioSpeech},
		{"audioTranscription", DirectProtocolAudioTranscription, e.AudioTranscription},
		{"audioTranslation", DirectProtocolAudioTranslation, e.AudioTranslation},
		{"moderations", DirectProtocolModerations, e.Moderations},
		{"video", DirectProtocolVideo, e.Video},
		{"geminiEmbeddings", DirectProtocolGeminiEmbeddings, e.GeminiEmbeddings},
		{"jinaEmbeddings", DirectProtocolJinaEmbeddings, e.JinaEmbeddings},
		{"modelscopeImageGeneration", DirectProtocolModelScopeImageGeneration, e.ModelScopeImageGeneration},
	}
}

func (e DirectEndpoints) ProtocolMask() int {
	mask := 0
	for _, entry := range e.Entries() {
		if entry.Endpoint != nil {
			mask |= entry.Protocol
		}
	}
	return mask
}

func (e DirectEndpoints) ForProtocol(bit int) *DirectEndpoint {
	for _, entry := range e.Entries() {
		if entry.Protocol == bit {
			return entry.Endpoint
		}
	}
	return nil
}
