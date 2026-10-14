package backup

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"slices"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// AxonHub gemini_vertex import (G batch): the GCP credential payload becomes a
// gcp_service_account credential, and endpoints derive the Vertex publisher
// path from the audited host shape plus the credential's project.

func vertexFixtureSA(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	sa := map[string]string{
		"type":         "service_account",
		"client_email": "vertex@fixture.iam.gserviceaccount.com",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":    "https://oauth2.googleapis.com/token",
	}
	raw, err := json.Marshal(sa)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func vertexFixturePayload(t *testing.T, gcp map[string]any) []byte {
	t.Helper()
	channel := map[string]any{
		"id": 1, "type": "gemini_vertex", "name": "vertex",
		"base_url":         "https://europe-west4-aiplatform.googleapis.com",
		"credentials":      map[string]any{"gcp": gcp},
		"supported_models": []string{"gemini-2.5-pro"},
		"settings":         map[string]any{"modelProtocols": []any{map[string]any{"model": "vertex", "apiFormats": []string{"gemini/contents", "gemini/embeddings"}}}},
	}
	models := []map[string]any{
		{"id": 1, "model_id": "gemini-2.5-pro", "type": "chat", "settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "gemini-2.5-pro"}}}}},
	}
	raw, err := json.Marshal(map[string]any{"version": "1.4", "channels": []map[string]any{channel}, "models": models})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAxonHubVertexImport(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := vertexFixturePayload(t, map[string]any{"region": "europe-west4", "projectID": "my-project", "jsonData": vertexFixtureSA(t)})

	preview, err := PreviewAxonHubV14(db, raw, "vertex")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blocking) != 0 || len(preview.SkippedChannels) != 0 {
		t.Fatalf("vertex preview: %+v", preview)
	}
	if _, err := ImportAxonHubV14(db, raw, "vertex", false); err != nil {
		t.Fatal(err)
	}

	var endpoints store.DirectEndpoints
	if err := db.Get(&endpoints, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE name='vertex'`)); err != nil {
		t.Fatal(err)
	}
	want := "https://europe-west4-aiplatform.googleapis.com/v1/projects/my-project/locations/europe-west4/publishers/google/models"
	if endpoints.Gemini == nil || endpoints.Gemini.URL != want || !endpoints.Gemini.ModelPath || endpoints.Gemini.Auth != store.DirectAuthBearer || endpoints.Gemini.Profile != "vertex" {
		t.Fatalf("vertex endpoint: %+v", endpoints.Gemini)
	}
	if endpoints.GeminiEmbeddings == nil || endpoints.GeminiEmbeddings.URL != want {
		t.Fatalf("vertex embeddings endpoint: %+v", endpoints.GeminiEmbeddings)
	}
	var kind, secret, material string
	if err := db.QueryRow(db.Rebind(`SELECT kind, secret, COALESCE(gcp_service_account,'') FROM upstream_credentials`)).Scan(&kind, &secret, &material); err != nil {
		t.Fatal(err)
	}
	if kind != store.DirectCredentialGCPServiceAccount || secret != "" || !strings.Contains(material, "vertex@fixture.iam.gserviceaccount.com") {
		t.Fatalf("credential: kind=%s secret=%q material=%d bytes", kind, secret, len(material))
	}
	// The grant binds the model to the GCP credential.
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_grants g JOIN upstream_credentials c ON c.id=g.credential_id WHERE c.kind=?`, store.DirectCredentialGCPServiceAccount) != 1 {
		t.Fatal("grant did not bind the GCP credential")
	}
}

func TestAxonHubVertexImportRejectsInvalidMaterial(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := vertexFixturePayload(t, map[string]any{"region": "europe-west4", "projectID": "my-project", "jsonData": "not-json"})
	preview, err := PreviewAxonHubV14(db, raw, "vertex")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.SkippedChannels) != 1 || !slices.Contains(preview.SkippedChannels[0].Reasons, "gcp_credential_invalid") {
		t.Fatalf("invalid material must be refused: %+v", preview.SkippedChannels)
	}
	if preview.Routable["channels"] != 0 {
		t.Fatalf("invalid vertex channel reached the plan: %+v", preview.Routable)
	}

	raw = vertexFixturePayload(t, map[string]any{"region": "europe-west4", "projectID": "", "jsonData": vertexFixtureSA(t)})
	preview, err = PreviewAxonHubV14(db, raw, "vertex-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.SkippedChannels) != 1 || !slices.Contains(preview.SkippedChannels[0].Reasons, "gcp_project_missing") {
		t.Fatalf("missing project must be refused: %+v", preview.SkippedChannels)
	}
}

func TestAxonHubVertexAnthropicImport(t *testing.T) {
	db := openAxonHubTestDB(t)
	channel := map[string]any{
		"id": 1, "type": "anthropic_gcp", "name": "vertex-claude",
		"base_url":         "https://us-east5-aiplatform.googleapis.com",
		"credentials":      map[string]any{"gcp": map[string]any{"region": "us-east5", "projectID": "my-project", "jsonData": vertexFixtureSA(t)}},
		"supported_models": []string{"claude-sonnet-4-5@20250929"},
		"settings":         map[string]any{"modelProtocols": []any{map[string]any{"model": "vertex-claude", "apiFormats": []string{"anthropic/messages"}}}},
	}
	models := []map[string]any{
		{"id": 1, "model_id": "claude-sonnet-4-5@20250929", "type": "chat", "settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "claude-sonnet-4-5@20250929"}}}}},
	}
	raw, err := json.Marshal(map[string]any{"version": "1.4", "channels": []map[string]any{channel}, "models": models})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewAxonHubV14(db, raw, "vertex-anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blocking) != 0 || len(preview.SkippedChannels) != 0 {
		t.Fatalf("anthropic vertex preview: %+v", preview)
	}
	if _, err := ImportAxonHubV14(db, raw, "vertex-anthropic", false); err != nil {
		t.Fatal(err)
	}
	var endpoints store.DirectEndpoints
	if err := db.Get(&endpoints, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE name='vertex-claude'`)); err != nil {
		t.Fatal(err)
	}
	want := "https://us-east5-aiplatform.googleapis.com/v1/projects/my-project/locations/us-east5/publishers/anthropic/models"
	if endpoints.Messages == nil || endpoints.Messages.URL != want || !endpoints.Messages.ModelPath || endpoints.Messages.Auth != store.DirectAuthBearer || endpoints.Messages.Profile != "vertex-anthropic" {
		t.Fatalf("vertex anthropic endpoint: %+v", endpoints.Messages)
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_credentials WHERE kind=? AND secret='' AND gcp_service_account <> ''`, store.DirectCredentialGCPServiceAccount) != 1 {
		t.Fatal("GCP credential not persisted with material")
	}
}

func TestAxonHubVertexCustomEndpointRefused(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := vertexFixturePayload(t, map[string]any{"region": "europe-west4", "projectID": "my-project", "jsonData": vertexFixtureSA(t)})
	// A custom endpoint on a vertex channel forces the Google API-key auth
	// shape that a service-account token cannot serve — refuse, honestly.
	var withCustom map[string]any
	if err := json.Unmarshal(raw, &withCustom); err != nil {
		t.Fatal(err)
	}
	withCustom["channels"].([]any)[0].(map[string]any)["endpoints"] = []any{map[string]any{"api_format": "gemini/contents", "path": "/custom"}}
	mutated, _ := json.Marshal(withCustom)
	preview, err := PreviewAxonHubV14(db, mutated, "vertex-custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.SkippedChannels) != 1 || !slices.Contains(preview.SkippedChannels[0].Reasons, "vertex_custom_endpoint_unsupported") {
		t.Fatalf("custom vertex endpoint must be refused: %+v", preview.SkippedChannels)
	}
}
