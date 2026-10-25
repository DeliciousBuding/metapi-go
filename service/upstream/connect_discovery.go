package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/internal/ssrf"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

const connectDiscoveryTimeout = 15 * time.Second
const connectDiscoveryMaxBytes = 16 << 20
const connectDiscoveryMaxModels = 10000

type modelCatalogEndpoint struct {
	address string
	auth    string
	format  string
}

// Catalog candidates come only from configured wire contracts. Shared URLs
// remain distinct when authentication or pagination formats differ.
func connectModelsEndpoints(endpoints store.DirectEndpoints) []modelCatalogEndpoint {
	var candidates []modelCatalogEndpoint
	seen := map[modelCatalogEndpoint]bool{}
	for _, entry := range endpoints.Entries() {
		candidate, ok := modelCatalogForEndpoint(entry)
		if ok && !seen[candidate] {
			seen[candidate] = true
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

// Native actions without a known catalog API do not invite URL probing.
func modelCatalogForEndpoint(entry store.DirectEndpointEntry) (modelCatalogEndpoint, bool) {
	ep := entry.Endpoint
	if ep == nil || ep.Profile == "bedrock" || ep.Profile == "vertex" || ep.Profile == "codex" || ep.Profile == "claudecode" || ep.Profile == "cline" || strings.HasPrefix(ep.Profile, "codex-") {
		return modelCatalogEndpoint{}, false
	}
	u, err := url.Parse(ep.URL)
	if err != nil {
		return modelCatalogEndpoint{}, false
	}
	if ep.Profile == "ollama" || ep.Profile == "ollama-messages" {
		for _, suffix := range []string{"/api/chat", "/v1/messages"} {
			if strings.HasSuffix(u.Path, suffix) {
				u.Path = strings.TrimSuffix(u.Path, suffix) + "/api/tags"
				return modelCatalogEndpoint{u.String(), ep.Auth, "ollama"}, true
			}
		}
	}
	if (entry.Protocol == store.DirectProtocolGemini || entry.Protocol == store.DirectProtocolGeminiEmbeddings) && ep.ModelPath {
		return modelCatalogEndpoint{ep.URL, ep.Auth, "gemini"}, true
	}
	for _, suffix := range []string{"/chat/completions", "/responses", "/messages", "/completions", "/embeddings", "/rerank", "/images/generations", "/videos"} {
		if strings.HasSuffix(u.Path, suffix) {
			u.Path = strings.TrimSuffix(u.Path, suffix) + "/models"
			format := "openai"
			if entry.Protocol == store.DirectProtocolMessages {
				format = "anthropic"
			}
			return modelCatalogEndpoint{u.String(), ep.Auth, format}, true
		}
	}
	return modelCatalogEndpoint{}, false
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
	if in.CredentialKind == store.DirectCredentialGCPServiceAccount {
		if _, err := modelCatalogCustomHeaders(in.Channel.CustomHeaders); err != nil {
			return nil, Discovery{}, err
		}
		// A new service account has durable material but no cached token.
		// Preserve lazy exchange: stored requests and catalog refresh resolve
		// it through the credential owner after this connection is created.
		models, discovery, err := presetConnectModels(in.RecommendedModels)
		if err == nil {
			discovery.Message = "Upstream model availability was not verified; refresh models after creating the connection to resolve the GCP service account credential"
		}
		return models, discovery, err
	}
	models, discovery, err := discoverUpstreamCatalog(ctx, in.Channel.Endpoints, func(context.Context) (string, error) {
		return in.Secret, nil
	}, in.Channel.ChannelProxy, in.Channel.CustomHeaders, in.Channel.UseSystemProxy)
	// Connect keeps the first-run UX fallbacks: no derivable catalog endpoint,
	// an endpoint that does not serve a catalog, or an authoritative empty
	// catalog all fall back to the preset list. Refresh (refresh_catalog.go)
	// uses the fallback-free core so an empty/missing catalog is an honest
	// answer, not a silently preserved preset.
	if errors.Is(err, errNoCatalogEndpoint) || errors.Is(err, errCatalogUnserved) || (err == nil && len(models) == 0) {
		return presetConnectModels(in.RecommendedModels)
	}
	return models, discovery, err
}

// errNoCatalogEndpoint: the channel's endpoints derive no model catalog URL.
// errCatalogUnserved: every candidate answers 404/405/501 on its first page.
var errNoCatalogEndpoint = errors.New("no derivable model catalog endpoint")
var errCatalogUnserved = errors.New("upstream endpoint does not serve a model catalog")

func modelCatalogCustomHeaders(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}
	var headers []struct {
		Key   string `json:"header_key"`
		Value string `json:"header_value"`
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&headers) != nil || headers == nil || decoder.Decode(&extra) != io.EOF {
		return nil, invalid("Invalid custom headers")
	}
	custom := make(map[string]string, len(headers))
	for _, header := range headers {
		if strings.TrimSpace(header.Key) == "" {
			return nil, invalid("Invalid custom headers")
		}
		if platform.IsDeniedCustomHeader(header.Key) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(header.Key)) {
		case "authorization", "x-api-key", "x-goog-api-key":
			// Match directProxyConfig: the selected endpoint owns credentials,
			// including their absence on an anonymous catalog request.
			continue
		}
		// Catalog discovery has no downstream client metadata to interpolate.
		value, ok := httpclient.ExpandClientHeaderTemplate(header.Value, nil)
		if !ok {
			return nil, invalid("Invalid custom headers")
		}
		custom[header.Key] = value
	}
	return custom, nil
}

// discoverUpstreamCatalog tries another configured catalog only when the first
// page is unserved. It never merges catalogs or substitutes preset models.
// An empty authoritative list is returned as ([],
// Discovery{Status:"empty"}, nil); the caller decides what an empty catalog
// means (connect falls back to presets, refresh reports it).
func discoverUpstreamCatalog(ctx context.Context, endpoints store.DirectEndpoints, credential func(context.Context) (string, error), channelProxy, rawCustomHeaders string, useSystemProxy bool) ([]string, Discovery, error) {
	ctx, cancel := context.WithTimeout(ctx, connectDiscoveryTimeout)
	defer cancel()
	customHeaders, err := modelCatalogCustomHeaders(rawCustomHeaders)
	if err != nil {
		return nil, Discovery{}, err
	}
	candidates := connectModelsEndpoints(endpoints)
	if len(candidates) == 0 {
		return nil, Discovery{}, errNoCatalogEndpoint
	}
	proxyURL := channelProxy
	if proxyURL == "" && useSystemProxy {
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
	// Each candidate is one exact URL; even same-origin redirects can reach a
	// different authenticated action. Never forward credentials on redirects.
	client := httpclient.NewClient(transport, connectDiscoveryTimeout, func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse })
	secret := ""
	for _, candidate := range candidates {
		u, err := url.Parse(candidate.address)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || ssrf.IsForbiddenSiteHostname(u.Hostname()) {
			return nil, Discovery{}, invalid("Invalid upstream model URL")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.address, nil)
		if err != nil {
			return nil, Discovery{}, invalid("Invalid upstream model URL")
		}
		request.Header.Set("Accept", "application/json")
		if candidate.format == "gemini" || candidate.format == "anthropic" {
			query := request.URL.Query()
			if candidate.format == "gemini" {
				query.Set("pageSize", "1000")
			} else {
				query.Set("limit", "1000")
			}
			request.URL.RawQuery = query.Encode()
		}
		// Resolve once, only if a catalog actually needs authentication. An
		// anonymous Ollama catalog must work without any stored secret, while
		// an authenticated fallback must never receive an empty credential.
		if candidate.auth != store.DirectAuthNone && secret == "" {
			secret, err = credential(ctx)
			if err != nil {
				return nil, Discovery{}, err
			}
			if secret == "" {
				return nil, Discovery{}, invalid("Channel has no enabled credential for model discovery")
			}
		}
		switch candidate.auth {
		case store.DirectAuthBearer:
			request.Header.Set("Authorization", "Bearer "+secret)
		case store.DirectAuthAPIKey:
			request.Header.Set("X-Api-Key", secret)
		case store.DirectAuthGoogle:
			request.Header.Set("X-Goog-Api-Key", secret)
		case store.DirectAuthNone:
		default:
			return nil, Discovery{}, invalid("Invalid model discovery authentication")
		}
		if candidate.format == "anthropic" {
			request.Header.Set("Anthropic-Version", "2023-06-01")
		}
		platform.ApplyCustomHeaders(request, customHeaders)
		models, discovery, err := discoverModelCatalog(client, request, candidate.format)
		if errors.Is(err, errCatalogUnserved) {
			continue
		}
		return models, discovery, err
	}
	return nil, Discovery{}, errCatalogUnserved
}

func discoverModelCatalog(client *http.Client, request *http.Request, format string) ([]string, Discovery, error) {
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
				return nil, Discovery{}, errCatalogUnserved
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
		return models, Discovery{Status: "empty", Message: "Upstream returned an empty model catalog"}, nil
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
