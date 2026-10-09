package backup

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func readAxonHubFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParseAxonHubSourceFixturePreservesImportRelevantFields(t *testing.T) {
	src, err := ParseAxonHubSource(readAxonHubFixture(t, "axonhub-basic.json"))
	if err != nil {
		t.Fatal(err)
	}
	if src.Version != "1.4" || len(src.Channels) != 1 || len(src.Models) != 1 || len(src.Projects) != 1 || len(src.APIKeys) != 1 {
		t.Fatalf("unexpected source counts: version=%q channels=%d models=%d projects=%d apiKeys=%d",
			src.Version, len(src.Channels), len(src.Models), len(src.Projects), len(src.APIKeys))
	}
	channel := src.Channels[0]
	if channel.Type != "openai" || channel.BaseURL != "https://api.example.invalid/v1" || channel.Credentials.APIKey != "fixture-api-key-primary" {
		t.Fatalf("channel source fields were not retained: %#v", channel)
	}
	if len(channel.Credentials.APIKeys) != 1 || channel.Credentials.OAuth {
		t.Fatalf("credential list was not retained: %#v", channel.Credentials)
	}
	if len(channel.Endpoints) != 1 || channel.Endpoints[0].APIFormat != "openai/chat_completions" {
		t.Fatalf("endpoint configuration was not retained: %#v", channel.Endpoints)
	}
	if len(channel.Settings.ModelMappings) != 1 || channel.Settings.ModelMappings[0].From != "public-model" {
		t.Fatalf("model mappings were not retained: %#v", channel.Settings.ModelMappings)
	}
	if src.APIKeys[0].ProjectID != 1 || src.APIKeys[0].AllowedIPs != 1 || !src.APIKeys[0].HasQuota {
		t.Fatalf("api key restrictions were not retained: %#v", src.APIKeys[0])
	}
	if src.Projects[0].ProfileRows != 1 {
		t.Fatalf("project profiles were not counted: %#v", src.Projects[0])
	}
	if len(src.ChannelModelPrices) != 1 || len(src.ChannelModelPrices[0].Price) == 0 {
		t.Fatal("channel pricing data was not retained")
	}
	if len(src.Models[0].Settings.Associations) != 1 || src.Models[0].Settings.Associations[0].ChannelModel == nil {
		t.Fatalf("model associations were not retained: %#v", src.Models[0].Settings)
	}
}

func TestParseAxonHubSourceCountsUnreviewedSections(t *testing.T) {
	payload := `{"version":"1.4","timestamp":"2026-01-01T00:00:00Z","channels":[],"models":[],"usage_requests":[],"future_section":[{"x":1},{"x":2}]}`
	src, err := ParseAxonHubSource([]byte(payload))
	if err != nil {
		t.Fatalf("ParseAxonHubSource: %v", err)
	}
	if src.UnknownSections["future_section"] != 1 {
		t.Fatalf("unreviewed section was not recorded: %#v", src.UnknownSections)
	}
}

func TestParseAxonHubSourceAllowsUnselectedNullSections(t *testing.T) {
	payload := `{"version":"1.4","channels":null,"models":null,"api_keys":null,"usage_requests":null}`
	src, err := ParseAxonHubSource([]byte(payload))
	if err != nil {
		t.Fatalf("ParseAxonHubSource: %v", err)
	}
	if len(src.Channels) != 0 || len(src.Models) != 0 {
		t.Fatalf("null source sections should represent unselected/empty data: %#v", src)
	}
}

func TestParseAxonHubSourcePreservesUnrecognizedProtocolForExplicitPlanning(t *testing.T) {
	payload := `{"version":"1.4","channels":[{"id":1,"type":"typesafe","endpoints":[{"api_format":"typesafe/system_one"}]}],"models":[]}`
	src, err := ParseAxonHubSource([]byte(payload))
	if err != nil {
		t.Fatalf("ParseAxonHubSource: %v", err)
	}
	if got := src.Channels[0].Endpoints[0].APIFormat; got != "typesafe/system_one" {
		t.Fatalf("source api_format = %q, want to preserve unrecognized target protocol", got)
	}
	if src.Channels[0].Type != "typesafe" {
		t.Fatalf("source provider type was not preserved: %q", src.Channels[0].Type)
	}
}

func TestParseAxonHubSourceRejectsMalformedOrUnreviewedBackups(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "bad json", body: `{"version":`, want: "invalid AxonHub backup JSON"},
		{name: "trailing value", body: `{"version":"1.4","channels":[],"models":[]} {}`, want: "invalid AxonHub backup JSON"},
		{name: "trailing garbage", body: `{"version":"1.4","channels":[],"models":[]} nope`, want: "invalid AxonHub backup JSON"},
		{name: "duplicate credential", body: `{"version":"1.4","channels":[{"id":1,"type":"openai","credentials":{"apiKey":"credential-poison-name","apiKey":"secret-poison-format"}}],"models":[]}`, want: "ambiguous or malformed"},
		{name: "duplicate version", body: `{"version":"1.4","version":"credential-poison-version","channels":[],"models":[]}`, want: "ambiguous or malformed"},
		{name: "unverified old version", body: `{"version":"1.3","channels":[],"models":[]}`, want: "unsupported AxonHub backup version"},
		{name: "future version", body: `{"version":"1.5","channels":[],"models":[]}`, want: "unsupported AxonHub backup version"},
		{name: "missing channels", body: `{"version":"1.4","models":[]}`, want: "missing channels field"},
		{name: "missing models", body: `{"version":"1.4","channels":[]}`, want: "missing models field"},
		{name: "invalid endpoint", body: `{"version":"1.4","channels":[{"id":1,"type":"openai","name":"credential-poison-name","endpoints":[{"api_format":"secret-poison-format"},{}]}],"models":[]}`, want: "channel[0].endpoints[1] missing api_format"},
		{name: "duplicate endpoint format", body: `{"version":"1.4","channels":[{"id":1,"type":"openai","name":"credential-poison-name","endpoints":[{"api_format":"secret-poison-format"},{"api_format":"secret-poison-format"}]}],"models":[]}`, want: "duplicate endpoint format at channel[0].endpoints[1]"},
		{name: "invalid version poison", body: `{"version":"credential-poison-version","channels":[],"models":[]}`, want: "unsupported AxonHub backup version (only 1.4 is supported)"},
		{name: "endpoint path traversal", body: `{"version":"1.4","channels":[{"id":1,"type":"openai","endpoints":[{"api_format":"openai/chat_completions","path":"/../etc/passwd"}]}],"models":[]}`, want: "invalid path"},
		{name: "duplicate model id", body: `{"version":"1.4","channels":[],"models":[{"id":1,"model_id":"m"},{"id":2,"model_id":"m"}]}`, want: "duplicate model_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAxonHubSource([]byte(tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseAxonHubSource error = %v, want containing %q", err, tc.want)
			}
			for _, poison := range []string{"credential-poison-name", "secret-poison-format", "credential-poison-version"} {
				if strings.Contains(err.Error(), poison) {
					t.Fatalf("error echoed untrusted field %q: %v", poison, err)
				}
			}
			var sourceErr AxonHubSourceError
			if !errors.As(err, &sourceErr) {
				t.Fatalf("error type = %T, want AxonHubSourceError", err)
			}
		})
	}
}

func TestParseAxonHubSourceDoesNotLeakCredentialValueInValidationError(t *testing.T) {
	payload := `{"version":"1.4","channels":[{"id":1,"type":"openai","endpoints":[{"api_format":""}],"credentials":{"apiKey":"never-echo-this"}}],"models":[]}`
	_, err := ParseAxonHubSource([]byte(payload))
	if err == nil {
		t.Fatal("ParseAxonHubSource succeeded, want malformed endpoint error")
	}
	if strings.Contains(err.Error(), "never-echo-this") {
		t.Fatalf("error leaked source credential: %v", err)
	}
}

func TestParseAxonHubSourceDoesNotEchoArbitraryChannelNames(t *testing.T) {
	payload := `{"version":"1.4","channels":[{"id":-1,"type":"openai","name":"private-customer-name"}],"models":[]}`
	_, err := ParseAxonHubSource([]byte(payload))
	if err == nil {
		t.Fatal("ParseAxonHubSource succeeded, want a non-positive id error")
	}
	if strings.Contains(err.Error(), "private-customer-name") {
		t.Fatalf("error echoed an operator-provided name: %v", err)
	}
}

func TestInspectExternalSourceRecognizesAxonHubV14(t *testing.T) {
	raw := readAxonHubFixture(t, "axonhub-basic.json")
	manifest, err := InspectExternalSource(raw)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Source != ExternalAxonHubV14 {
		t.Fatalf("source = %q, want %q", manifest.Source, ExternalAxonHubV14)
	}
	if manifest.Sections["channels"] != 1 || manifest.Sections["models"] != 1 {
		t.Fatalf("unexpected manifest sections: %#v", manifest.Sections)
	}
}

func TestIsAxonHubV14PayloadDistinguishesSources(t *testing.T) {
	if !IsAxonHubV14Payload(readAxonHubFixture(t, "axonhub-basic.json")) {
		t.Fatal("fixture was not detected as an AxonHub v1.4 payload")
	}
	if IsAxonHubV14Payload([]byte(`{"version":"5","exported_at":"x","channels":[]}`)) {
		t.Fatal("Octopus payload was misdetected as AxonHub")
	}
	// A v1.4 envelope without the required configuration sections is not a
	// payload this endpoint should route to the AxonHub importer.
	if IsAxonHubV14Payload([]byte(`{"version":"1.4","timestamp":"t"}`)) {
		t.Fatal("metadata-only 1.4 payload was misdetected as an AxonHub backup")
	}
}

func TestAxonHubPreviewRedactsSourceValues(t *testing.T) {
	raw := readAxonHubFixture(t, "axonhub-basic.json")
	preview, err := PreviewAxonHubV14(nil, raw, "fixture-origin")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Routable["channels"] != 1 || preview.Routable["routes"] != 1 {
		t.Fatalf("unexpected routable counts: %#v", preview.Routable)
	}
	if preview.NotImported["projects"] != 1 || preview.NotImported["apiKeys"] != 1 {
		t.Fatalf("unmanaged sections were not reported: %#v", preview.NotImported)
	}
	encoded, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fixture-api-key-primary", "fixture-api-key-secondary", "fixture-downstream-key"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("preview leaked a credential value: %s", secret)
		}
	}
	for _, private := range []string{"Fixture compatible provider", "example.invalid", "fixture-tag", "sanitized fixture"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("preview leaked source text %q", private)
		}
	}
}
