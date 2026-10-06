package backup

import (
	"encoding/json"
	"strings"
	"testing"
)

const octopusV5Fixture = `{
  "version":5,"exported_at":"2026-10-05T00:00:00Z",
  "channels":[{"id":1,"name":"Example","base_url":"https://example.com","dialect":"generic"}],
  "channel_keys":[{"id":2,"channel_id":1,"name":"primary","key":"sk-fixture-only","enabled":true}],
  "channel_models":[{"id":3,"channel_id":1,"name":"test-model"}],
  "channel_grants":[{"id":4,"channel_model_id":3,"channel_key_id":2,"protocols":14}],
  "groups":[{"id":5,"name":"test-model","mode":"failover"}],
  "group_items":[{"id":6,"group_id":5,"channel_grant_id":4,"priority":0}],
  "api_keys":[{"id":7,"name":"client","api_key":"client-fixture-key","supported_models":["test-model"]}]
}`

func TestInspectExternalSourceOctopusV5PreservesGraphCountsWithoutSecrets(t *testing.T) {
	got, err := InspectExternalSource([]byte(octopusV5Fixture))
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != ExternalOctopusV5 || got.Sections["channelGrants"] != 1 || got.Sections["groupItems"] != 1 || got.Sections["api_keys"] != 1 {
		t.Fatalf("wrong v5 manifest: %+v", got)
	}
	manifestJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifestJSON), "sk-") {
		t.Fatal("source manifest exposed credential material")
	}
}

func TestInspectExternalSourceRejectsBrokenOctopusReferences(t *testing.T) {
	for name, payload := range map[string]string{
		"cross-channel grant": strings.Replace(octopusV5Fixture, `"channel_model_id":3`, `"channel_model_id":999`, 1),
		"mismatched channel":  strings.Replace(strings.Replace(octopusV5Fixture, `"channels":[{"id":1,`, `"channels":[{"id":9},{"id":1,`, 1), `"channel_models":[{"id":3,"channel_id":1`, `"channel_models":[{"id":3,"channel_id":9`, 1),
		"missing group grant": strings.Replace(octopusV5Fixture, `"channel_grant_id":4`, `"channel_grant_id":999`, 1),
		"unknown protocol":    strings.Replace(octopusV5Fixture, `"protocols":14`, `"protocols":30`, 1),
		"duplicate key id":    strings.Replace(octopusV5Fixture, `"enabled":true}],`, `"enabled":true},{"id":2,"channel_id":1,"name":"duplicate"}],`, 1),
		"duplicate JSON key":  strings.Replace(octopusV5Fixture, `"name":"primary","key"`, `"name":"primary","name":"other","key"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := InspectExternalSource([]byte(payload))
			if err == nil {
				t.Fatal("broken graph was accepted")
			}
			if strings.Contains(err.Error(), "sk-fixture") {
				t.Fatal("error leaked credential")
			}
		})
	}
}

func TestInspectExternalSourceAxonHubV14AndUnknown(t *testing.T) {
	got, err := InspectExternalSource([]byte(`{"version":"1.4","timestamp":"2026-10-05T00:00:00Z","channels":[{"id":1,"type":"openai","credentials":{"api_keys":["sk-fixture-only"]}}],"models":[{"model_id":"test-model"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != ExternalAxonHubV14 || got.Sections["channels"] != 1 || got.Sections["models"] != 1 {
		t.Fatalf("wrong AxonHub manifest: %+v", got)
	}
	for _, raw := range []string{
		`{"version":2,"exported_at":"2026-08-01T00:00:00Z","channels":[{"id":1}]}`,
		`{"version":"1.4","timestamp":"2026-10-05T00:00:00Z","channels":[{"id":1}]}`,
		`{"version":5,"exported_at":"2026-10-05T00:00:00Z","channels":[]}`,
		`{"version":5,"exported_at":"2026-10-05T00:00:00Z","channels":null}`,
	} {
		if _, err := InspectExternalSource([]byte(raw)); err == nil {
			t.Fatalf("invalid source accepted: %s", raw)
		}
	}
}
