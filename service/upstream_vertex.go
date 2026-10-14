package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// Vertex AI endpoint derivation (G batch).
//
// A Vertex channel's executable URL is
// {base}/v1/projects/{project}/locations/{region}/publishers/google/models
// with ModelPath appending {model}:{action}. The region comes from the base
// host (the {region}-aiplatform.googleapis.com shape, or the global
// aiplatform.googleapis.com endpoint); the project comes from the operator.
// Both substitutions are validated here — the preset stores only templates,
// never a partially formed URL.

var (
	vertexRegionalHost = regexp.MustCompile(`^([a-z0-9-]+)-aiplatform\.googleapis\.com$`)
	vertexProjectID    = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	vertexProjectNum   = regexp.MustCompile(`^[0-9]{6,15}$`)
)

// VertexProjectRule is the operator-facing validation message.
const VertexProjectRule = "GCP project ID (6-30 lowercase letters, digits, hyphens) or project number"

// vertexLocationFromBaseURL extracts the Vertex location from the regional
// host shape; the bare global host maps to location "global".
func vertexLocationFromBaseURL(base string) (string, error) {
	host := strings.ToLower(strings.TrimSpace(base))
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if i := strings.IndexAny(host, "/"); i >= 0 {
		host = host[:i]
	}
	if host == "aiplatform.googleapis.com" {
		return "global", nil
	}
	if m := vertexRegionalHost.FindStringSubmatch(host); m != nil {
		return m[1], nil
	}
	return "", fmt.Errorf("Vertex presets need a {region}-aiplatform.googleapis.com endpoint; for custom domains configure the endpoint URL manually")
}

// VertexEndpointURL composes the publisher-models prefix for one Vertex
// channel base URL and project. ModelPath appends {model}:{action} to it.
func VertexEndpointURL(baseURL, project, publisher string) (string, error) {
	project = strings.TrimSpace(project)
	if publisher != "google" && publisher != "anthropic" {
		return "", fmt.Errorf("unsupported Vertex publisher %q", publisher)
	}
	if !vertexProjectID.MatchString(project) && !vertexProjectNum.MatchString(project) {
		return "", fmt.Errorf("Invalid GCP project: %s", VertexProjectRule)
	}
	location, err := vertexLocationFromBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(baseURL, "/") + "/v1/projects/" + project + "/locations/" + location + "/publishers/" + publisher + "/models", nil
}

// DeriveVertexEndpoints substitutes project and region into the vertex
// endpoint templates of the preset config. Every URL field is visited; the
// result contains no placeholders.
func DeriveVertexEndpoints(config store.DirectEndpoints, baseURL, project string) (store.DirectEndpoints, error) {
	project = strings.TrimSpace(project)
	if !vertexProjectID.MatchString(project) && !vertexProjectNum.MatchString(project) {
		return config, fmt.Errorf("Invalid GCP project: %s", VertexProjectRule)
	}
	location, err := vertexLocationFromBaseURL(baseURL)
	if err != nil {
		return config, err
	}
	for _, entry := range config.Entries() {
		if entry.Endpoint == nil {
			continue
		}
		for _, field := range entry.Endpoint.URLFields() {
			*field = strings.ReplaceAll(*field, "{project}", project)
			*field = strings.ReplaceAll(*field, "{region}", location)
		}
		if strings.Contains(entry.Endpoint.URL, "{") {
			return config, fmt.Errorf("vertex endpoint template retains an unbound placeholder")
		}
	}
	return config, nil
}
