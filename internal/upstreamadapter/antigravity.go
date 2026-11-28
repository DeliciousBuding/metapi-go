package upstreamadapter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// These wire defaults follow the audited Antigravity installed-client contract.
// Version discovery and endpoint retry are deliberately outside a pure adapter.
const antigravityWireUserAgent = "antigravity/1.20.4 windows/amd64"

func prepareAntigravity(in Input, wire *Wire) error {
	if !in.OAuth || strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.ActualModel) == "" {
		return fmt.Errorf("Antigravity requires an OAuth project and model")
	}
	request, err := antigravityObject(in.Body)
	if err != nil {
		return fmt.Errorf("invalid Antigravity Gemini request")
	}
	if err := adaptAntigravityRequest(request, in.ActualModel); err != nil {
		return err
	}
	wire.Body, err = json.Marshal(map[string]any{
		"project": in.ProjectID, "model": antigravityModel(in.ActualModel),
		"request": request, "requestType": "agent", "userAgent": "antigravity",
		"requestId": "agent-" + uuid.NewString(),
	})
	if err != nil {
		return fmt.Errorf("cannot encode Antigravity request")
	}
	if wire.Headers == nil {
		wire.Headers = make(http.Header)
	}
	wire.Headers.Set("Content-Type", "application/json")
	wire.Headers.Set("User-Agent", antigravityWireUserAgent)
	wire.Headers.Set("X-Goog-Api-Client", "google-cloud-sdk vscode_cloudshelleditor/0.1")
	wire.Headers.Set("Client-Metadata", `{"ideType":"ANTIGRAVITY","platform":"PLATFORM_UNSPECIFIED","pluginType":"GEMINI"}`)
	wire.Headers.Set("X-Opencode-Tools-Debug", "1")
	wire.Headers.Set("Accept", "application/json")
	if in.Stream {
		wire.Headers.Set("Accept", "text/event-stream")
	}
	// Generic authentication is the caller's responsibility. In particular,
	// retain its freshly resolved Bearer header; never copy downstream auth.
	wire.RemoveHeaders = append(wire.RemoveHeaders, "X-Goog-Api-Key", "X-Api-Key",
		"User-Agent", "X-Goog-Api-Client", "Client-Metadata")
	return nil
}

func antigravityModel(model string) string {
	model = strings.TrimSuffix(strings.TrimSuffix(model, ":antigravity"), ":gemini-cli")
	model = strings.TrimPrefix(model, "antigravity-")
	lower := strings.ToLower(model)
	if strings.HasPrefix(lower, "gemini-3-pro") &&
		!strings.HasSuffix(lower, "-low") && !strings.HasSuffix(lower, "-medium") && !strings.HasSuffix(lower, "-high") {
		model += "-low"
	}
	return model
}
