package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDirectMediaProfileContracts(t *testing.T) {
	tests := []struct {
		name, key, profile, auth, model string
		modelPath, valid                bool
	}{
		{"jina", "jinaEmbeddings", "jina-embeddings", DirectAuthBearer, "", false, true},
		{"missing jina profile", "jinaEmbeddings", "", DirectAuthBearer, "", false, false},
		{"jina wrong slot", "embeddings", "jina-embeddings", DirectAuthBearer, "", false, false},
		{"modelscope", "modelscopeImageGeneration", "modelscope-image", DirectAuthBearer, "", false, true},
		{"missing modelscope profile", "modelscopeImageGeneration", "", DirectAuthBearer, "", false, false},
		{"modelscope edit", "imageEdit", "modelscope-image", DirectAuthBearer, "", false, true},
		{"minimax", "imageGeneration", "minimax-image", DirectAuthBearer, "", false, true},
		{"minimax edit unsupported", "imageEdit", "minimax-image", DirectAuthBearer, "", false, false},
		{"codex", "imageGeneration", "codex-image", DirectAuthBearer, "gpt-6-luna", false, true},
		{"codex no request model", "imageEdit", "codex-image", DirectAuthBearer, "", false, false},
		{"codex blank request model", "imageEdit", "codex-image", DirectAuthBearer, "  ", false, false},
		{"codex long request model", "imageEdit", "codex-image", DirectAuthBearer, strings.Repeat("x", 256), false, false},
		{"codex wrong auth", "imageGeneration", "codex-image", DirectAuthAPIKey, "gpt-6-luna", false, false},
		{"ordinary model override", "imageGeneration", "", DirectAuthBearer, "gpt-6-luna", false, false},
		{"native gemini", "geminiEmbeddings", "", DirectAuthGoogle, "", true, true},
		{"ordinary model path", "embeddings", "", DirectAuthBearer, "", true, false},
		{"ollama bearer", "ollama", "ollama", DirectAuthBearer, "", false, true},
		{"ollama anonymous", "ollama", "ollama", DirectAuthNone, "", false, true},
		{"ollama missing profile", "ollama", "", DirectAuthBearer, "", false, false},
		{"ollama messages", "messages", "ollama-messages", DirectAuthNone, "", false, true},
		{"no anonymous generic endpoint", "chat", "", DirectAuthNone, "", false, false},
		{"bedrock model prefix", "messages", "bedrock", DirectAuthBearer, "", true, true},
		{"bedrock missing model prefix", "messages", "bedrock", DirectAuthBearer, "", false, false},
		{"seedance", "seedanceVideo", "seedance-video", DirectAuthBearer, "", false, true},
		{"zenmux video", "zenmuxVideo", "zenmux-video", DirectAuthBearer, "", false, true},
		{"native video wrong slot", "video", "seedance-video", DirectAuthBearer, "", false, false},
		{"codex search", "alphaSearch", "codex-alpha-search", DirectAuthBearer, "", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]DirectEndpoint{tc.key: {URL: "https://provider.example/endpoint", Profile: tc.profile, Auth: tc.auth, RequestModel: tc.model, ModelPath: tc.modelPath}})
			if err != nil {
				t.Fatal(err)
			}
			var endpoints DirectEndpoints
			if err := endpoints.Scan(raw); (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
		})
	}
}

func TestDirectMediaProtocolOrderRetainsIndependentFormats(t *testing.T) {
	want := DirectProtocolOrder{DirectProtocolJinaEmbeddings, DirectProtocolEmbeddings, DirectProtocolModelScopeImageGeneration, DirectProtocolImageGeneration}
	raw, err := want.Value()
	if err != nil {
		t.Fatal(err)
	}
	var loaded DirectProtocolOrder
	if err := loaded.Scan(raw); err != nil {
		t.Fatal(err)
	}
	if len(loaded) != len(want) {
		t.Fatal("format collapsed")
	}
	for i, bit := range want {
		if loaded[i] != bit {
			t.Fatal("order changed")
		}
	}
	for _, invalid := range []string{"[64,64]", "[1]", "[16777216]", "[192]"} {
		if loaded.Scan(invalid) == nil {
			t.Fatalf("accepted invalid order %s", invalid)
		}
	}
}
