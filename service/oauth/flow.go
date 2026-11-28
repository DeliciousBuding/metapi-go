package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

const manualCallbackDelayMs = 15000

// FlowStartResult is the result of starting an OAuth flow.
type FlowStartResult struct {
	Provider         string            `json:"provider"`
	State            string            `json:"state"`
	AuthorizationURL string            `json:"authorizationUrl"`
	Instructions     *FlowInstructions `json:"instructions"`
}

// FlowInstructions holds callback instructions for the user.
type FlowInstructions struct {
	RedirectURI           string                  `json:"redirectUri"`
	CallbackPort          int                     `json:"callbackPort"`
	CallbackPath          string                  `json:"callbackPath"`
	ManualCallbackDelayMs int                     `json:"manualCallbackDelayMs"`
	SSHTunnelCommand      string                  `json:"sshTunnelCommand,omitempty"`
	SSHTunnelKeyCommand   string                  `json:"sshTunnelKeyCommand,omitempty"`
	DeviceFlow            *DeviceFlowInstructions `json:"deviceFlow,omitempty"`
}

// StartFlowInput holds the input for starting an OAuth flow.
type StartFlowInput struct {
	Provider        string
	RebindAccountID int64
	ProjectID       string
	ProxyURL        string
	UseSystemProxy  bool
	RequestOrigin   string
}

var startLoopbackCallbackServerForFlow = StartLoopbackCallbackServer

// StartFlow initiates an OAuth flow for the given provider.
func StartFlow(db *store.DB, input StartFlowInput) (*FlowStartResult, error) {
	def := GetProviderDefinition(input.Provider)
	if def == nil {
		return nil, fmt.Errorf("unsupported oauth provider: %s", input.Provider)
	}

	callbackState, callbackErr := startLoopbackCallbackServerForFlow(input.Provider)
	if callbackErr != nil || callbackState == nil || !callbackState.Ready {
		callbackError := "listener did not become ready"
		if callbackErr != nil {
			callbackError = callbackErr.Error()
		} else if callbackState != nil && callbackState.Error != "" {
			callbackError = callbackState.Error
		}
		slog.Warn("oauth callback listener unavailable; manual callback remains available",
			"provider", input.Provider,
			"error", callbackError)
	}

	redirectURI := def.Loopback.RedirectURI
	session, err := CreateSession(CreateSessionInput{
		db:              db,
		Provider:        input.Provider,
		RedirectURI:     redirectURI,
		RebindAccountID: input.RebindAccountID,
		ProjectID:       input.ProjectID,
		ProxyURL:        input.ProxyURL,
		UseSystemProxy:  input.UseSystemProxy,
		SessionTTL:      def.SessionTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	authURL, err := def.BuildAuthorizationURL(context.Background(), BuildAuthURLInput{
		State:        session.State,
		RedirectURI:  session.RedirectURI,
		CodeVerifier: session.CodeVerifier,
		ProjectID:    session.ProjectID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build authorization URL: %w", err)
	}

	instructions := buildLoopbackInstructions(def, input.RequestOrigin)
	if def.Metadata.SupportsDeviceFlow {
		instructions.DeviceFlow = deviceFlowInstructions(session.State)
	}

	return &FlowStartResult{
		Provider:         input.Provider,
		State:            session.State,
		AuthorizationURL: authURL,
		Instructions:     instructions,
	}, nil
}

func isLoopbackHost(hostname string) bool {
	normalized := strings.ToLower(strings.TrimSpace(hostname))
	return normalized == "localhost" ||
		normalized == "127.0.0.1" ||
		normalized == "::1" ||
		normalized == "[::1]"
}

func resolveSSHTunnelHost(requestOrigin string) string {
	if requestOrigin == "" {
		return ""
	}
	parsed, err := url.Parse(requestOrigin)
	if err != nil {
		return ""
	}
	if parsed.Hostname() == "" || isLoopbackHost(parsed.Hostname()) {
		return ""
	}
	return parsed.Hostname()
}

func buildLoopbackInstructions(def *OAuthProviderDefinition, requestOrigin string) *FlowInstructions {
	sshHost := resolveSSHTunnelHost(requestOrigin)
	instructions := &FlowInstructions{
		RedirectURI:           def.Loopback.RedirectURI,
		CallbackPort:          def.Loopback.Port,
		CallbackPath:          def.Loopback.Path,
		ManualCallbackDelayMs: manualCallbackDelayMs,
	}
	if sshHost != "" {
		instructions.SSHTunnelCommand = fmt.Sprintf("ssh -L %d:127.0.0.1:%d root@%s -p 22",
			def.Loopback.Port, def.Loopback.Port, sshHost)
		instructions.SSHTunnelKeyCommand = fmt.Sprintf("ssh -i <path_to_your_key> -L %d:127.0.0.1:%d root@%s -p 22",
			def.Loopback.Port, def.Loopback.Port, sshHost)
	}
	return instructions
}

// ListOauthProviders returns provider metadata with callback server state.
func ListOauthProviders() []ProviderMetadata {
	defs := ListProviderDefinitions()
	result := make([]ProviderMetadata, len(defs))
	for i, def := range defs {
		meta := def.Metadata
		state := GetLoopbackCallbackServerState(string(def.Metadata.Provider))
		if state != nil {
			meta.Enabled = state.Ready || !state.Attempted
		}
		result[i] = meta
	}
	return result
}
