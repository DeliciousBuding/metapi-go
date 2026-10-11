package upstream

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/internal/ssrf"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

const connectDiscoveryTimeout = 15 * time.Second
const connectDiscoveryMaxBytes = 16 << 20
const connectDiscoveryMaxModels = 10000

// A models endpoint is derived only from a known wire contract. Native actions
// without a catalog API use preset models instead of speculative URL probing.
func connectModelsEndpoint(endpoints store.DirectEndpoints) (string, string, string) {
	for _, entry := range endpoints.Entries() {
		ep := entry.Endpoint
		if ep == nil {
			continue
		}
		if ep.Profile == "bedrock" || ep.Profile == "codex" || ep.Profile == "claudecode" || ep.Profile == "cline" || strings.HasPrefix(ep.Profile, "codex-") {
			continue
		}
		u, err := url.Parse(ep.URL)
		if err != nil {
			continue
		}
		if ep.Profile == "ollama" || ep.Profile == "ollama-messages" {
			for _, suffix := range []string{"/api/chat", "/v1/messages"} {
				if strings.HasSuffix(u.Path, suffix) {
					u.Path = strings.TrimSuffix(u.Path, suffix) + "/api/tags"
					return u.String(), ep.Auth, "ollama"
				}
			}
		}
		if (entry.Protocol == store.DirectProtocolGemini || entry.Protocol == store.DirectProtocolGeminiEmbeddings) && ep.ModelPath {
			return ep.URL, ep.Auth, "gemini"
		}
		for _, suffix := range []string{"/chat/completions", "/responses", "/messages", "/completions", "/embeddings", "/rerank", "/images/generations", "/videos"} {
			if strings.HasSuffix(u.Path, suffix) {
				u.Path = strings.TrimSuffix(u.Path, suffix) + "/models"
				format := "openai"
				if entry.Protocol == store.DirectProtocolMessages {
					format = "anthropic"
				}
				return u.String(), ep.Auth, format
			}
		}
	}
	return "", "", ""
}

func connectDiscoveryError(message string) error {
	return &Error{Status: http.StatusBadGateway, Message: message}
}

func normalizeConnectModels(names []string) ([]string, error) {
	if len(names) > connectDiscoveryMaxModels {
		return nil, connectDiscoveryError("Upstream model catalog exceeds the 10000 model limit")
	}
	models := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if !validName(name) || !routing.IsExactRouteModelPattern(name) {
			return nil, connectDiscoveryError("Upstream model catalog contains an invalid model name")
		}
		if !seen[name] {
			seen[name] = true
			models = append(models, name)
		}
	}
	slices.Sort(models)
	return models, nil
}

func presetConnectModels(names []string) ([]string, Discovery, error) {
	models, err := normalizeConnectModels(names)
	if err != nil {
		return nil, Discovery{}, err
	}
	if len(models) == 0 {
		return models, Discovery{Status: "empty", Message: "No models discovered; add models to finish configuration"}, nil
	}
	return models, Discovery{Status: "preset", Message: "Using preset models; upstream model availability was not verified"}, nil
}

func discoverConnectModels(ctx context.Context, in ConnectInput) ([]string, Discovery, error) {
	ctx, cancel := context.WithTimeout(ctx, connectDiscoveryTimeout)
	defer cancel()
	address, auth, format := connectModelsEndpoint(in.Channel.Endpoints)
	if address == "" {
		return presetConnectModels(in.RecommendedModels)
	}
	u, err := url.Parse(address)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || ssrf.IsForbiddenSiteHostname(u.Hostname()) {
		return nil, Discovery{}, invalid("Invalid upstream model URL")
	}
	proxyURL := in.Channel.ChannelProxy
	if proxyURL == "" && in.Channel.UseSystemProxy {
		if runtime := config.RuntimeSafe(); runtime != nil {
			proxyURL = runtime.SystemProxyUrl
		}
	}
	proxy := httpclient.NoProxy
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" || ssrf.IsForbiddenSiteHostname(parsed.Hostname()) || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5" && parsed.Scheme != "socks5h") {
			return nil, Discovery{}, invalid("Invalid channel proxy URL")
		}
		proxy = http.ProxyURL(parsed)
	}
	transport := httpclient.NewTransport(httpclient.Options{Proxy: proxy, SiteDialGuard: true, DialTimeout: 5 * time.Second, ResponseHeaderTimeout: connectDiscoveryTimeout})
	defer transport.CloseIdleConnections()
	// Discovery is one exact URL; even same-origin redirects can reach a
	// different authenticated action. Never forward credentials on redirects.
	client := httpclient.NewClient(transport, connectDiscoveryTimeout, func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse })
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, Discovery{}, invalid("Invalid upstream model URL")
	}
	request.Header.Set("Accept", "application/json")
	if format == "gemini" || format == "anthropic" {
		query := request.URL.Query()
		if format == "gemini" {
			query.Set("pageSize", "1000")
		} else {
			query.Set("limit", "1000")
		}
		request.URL.RawQuery = query.Encode()
	}
	switch auth {
	case store.DirectAuthBearer:
		request.Header.Set("Authorization", "Bearer "+in.Secret)
	case store.DirectAuthAPIKey:
		request.Header.Set("X-Api-Key", in.Secret)
	case store.DirectAuthGoogle:
		request.Header.Set("X-Goog-Api-Key", in.Secret)
	case store.DirectAuthNone:
	default:
		return nil, Discovery{}, invalid("Invalid model discovery authentication")
	}
	if format == "anthropic" {
		request.Header.Set("Anthropic-Version", "2023-06-01")
	}
	names := []string{}
	remainingBytes := connectDiscoveryMaxBytes
	seenCursors := map[string]bool{}
	for page := 0; ; page++ {
		catalog, status, consumed, err := fetchConnectCatalog(client, request, remainingBytes)
		if err != nil {
			return nil, Discovery{}, err
		}
		if status == http.StatusNotFound || status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
			if page == 0 {
				return presetConnectModels(in.RecommendedModels)
			}
			return nil, Discovery{}, connectDiscoveryError("Upstream model pagination failed")
		}
		remainingBytes -= consumed
		if format == "gemini" || format == "ollama" {
			if catalog.Models == nil {
				return nil, Discovery{}, connectDiscoveryError("Upstream returned an invalid model catalog")
			}
			for _, model := range *catalog.Models {
				name := model.Name
				if name == "" && format == "ollama" {
					name = model.Model
				}
				if format == "gemini" {
					name = strings.TrimPrefix(name, "models/")
				}
				names = append(names, name)
			}
		} else {
			if catalog.Data == nil {
				return nil, Discovery{}, connectDiscoveryError("Upstream returned an invalid model catalog")
			}
			for _, model := range *catalog.Data {
				names = append(names, model.ID)
			}
		}
		if len(names) > connectDiscoveryMaxModels {
			return nil, Discovery{}, connectDiscoveryError("Upstream model catalog exceeds the 10000 model limit")
		}
		if !catalog.HasMore && catalog.NextPageToken == "" {
			break
		}
		cursor, field := "", ""
		if format == "gemini" && catalog.NextPageToken != "" && !catalog.HasMore {
			cursor, field = catalog.NextPageToken, "pageToken"
		}
		if format == "anthropic" && catalog.HasMore && catalog.NextPageToken == "" {
			cursor, field = catalog.LastID, "after_id"
		}
		if cursor == "" || len(cursor) > 4096 || seenCursors[cursor] || page >= 99 {
			return nil, Discovery{}, connectDiscoveryError("Upstream returned unsupported or repeated model pagination")
		}
		seenCursors[cursor] = true
		// Only the cursor changes. Upstream metadata can never supply a new
		// origin or path to receive this connection's credential.
		query := request.URL.Query()
		query.Set(field, cursor)
		request.URL.RawQuery = query.Encode()
	}
	models, err := normalizeConnectModels(names)
	if err != nil {
		return nil, Discovery{}, err
	}
	if len(models) == 0 {
		return presetConnectModels(in.RecommendedModels)
	}
	return models, Discovery{Status: "discovered"}, nil
}

type connectModelCatalog struct {
	HasMore       bool   `json:"has_more"`
	LastID        string `json:"last_id"`
	NextPageToken string `json:"nextPageToken"`
	Data          *[]struct {
		ID string `json:"id"`
	} `json:"data"`
	Models *[]struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}

func fetchConnectCatalog(client *http.Client, request *http.Request, remainingBytes int) (connectModelCatalog, int, int, error) {
	var catalog connectModelCatalog
	response, err := client.Do(request)
	if err != nil {
		return catalog, 0, 0, connectDiscoveryError("Could not fetch upstream models; check the address and proxy settings")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return catalog, 0, 0, invalid("Upstream rejected the credential; check its permissions")
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return catalog, response.StatusCode, 0, nil
	}
	if response.StatusCode != http.StatusOK {
		return catalog, 0, 0, connectDiscoveryError("Upstream model discovery failed")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(remainingBytes)+1))
	if err != nil {
		return catalog, 0, 0, connectDiscoveryError("Could not read the upstream model catalog")
	}
	if len(body) > remainingBytes {
		return catalog, 0, 0, connectDiscoveryError("Upstream model catalog exceeds the response size limit")
	}
	if json.Unmarshal(body, &catalog) != nil {
		return catalog, 0, 0, connectDiscoveryError("Upstream returned an invalid model catalog")
	}
	return catalog, response.StatusCode, len(body), nil
}
