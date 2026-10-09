package backup

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func axonHubSettingsPayload(settings string) []byte {
	return []byte(fmt.Sprintf(`{"version":"1.4","timestamp":"2026-10-09T00:00:00Z","channels":[
		{"id":1,"type":"openai","name":"fixture","base_url":"https://relay.invalid","credentials":{"apiKey":"fixture-key"},"supported_models":["gpt-6"],"settings":%s}
	],"models":[{"id":1,"model_id":"gpt-6","type":"chat","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"gpt-6"}}]}}]}`, settings))
}

func TestAxonHubOfficialExportDefaultsImport(t *testing.T) {
	// BackupChannel/BackupModel embed ent values: edges is serialized even
	// without eager loading, and ChannelSettings always emits TransformOptions.
	raw := axonHubSettingsPayload(`{"extraModelPrefix":"","autoTrimedModelPrefixes":null,"modelMappings":null,"hideOriginalModels":false,"hideMappedModels":false,"lowercaseModelId":false,"overrideParameters":"","overrideHeaders":null,"transformOptions":{"forceArrayInstructions":false,"forceArrayInputs":false,"replaceDeveloperRoleWithSystem":false}}`)
	raw = []byte(strings.ReplaceAll(string(raw), `"id":1,`, `"edges":{},"id":1,`))
	db := openAxonHubTestDB(t)
	counts, err := ImportAxonHubV14(db, raw, "official-defaults", false)
	if err != nil {
		t.Fatal(err)
	}
	if counts["channels"] != 1 || counts["routes"] != 1 {
		t.Fatalf("official default export lost its graph: %#v", counts)
	}
}

func TestAxonHubSettingsExecutionSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, settings, override, blocked string
	}{
		{"disabled transforms", `{"transformOptions":{"forceArrayInstructions":false,"forceArrayInputs":false,"replaceDeveloperRoleWithSystem":false,"reasoningEffortMapping":[]}}`, "", ""},
		{"enabled transform", `{"transformOptions":{"forceArrayInputs":true}}`, "", "channel_transform_options_unsupported"},
		{"unknown disabled transform", `{"transformOptions":{"futureTransform":false}}`, "", "channel_transform_options_unsupported"},
		{"reasoning mapping", `{"transformOptions":{"reasoningEffortMapping":[{"from":"high","to":"max"}]}}`, "", "channel_transform_options_unsupported"},
		{"empty operations supersede legacy parameters", `{"overrideParameters":"{\"temperature\":0.1}","bodyOverrideOperations":[]}`, "", ""},
		{"null operations retain legacy parameters", `{"overrideParameters":"{\"temperature\":0.1}","bodyOverrideOperations":null}`, `{"temperature":0.1}`, ""},
		{"empty operations supersede legacy headers", `{"overrideHeaders":[{"key":"x-test","value":"old"}],"headerOverrideOperations":[]}`, "", ""},
		{"null operations retain legacy headers", `{"overrideHeaders":[{"key":"x-test","value":"old"}],"headerOverrideOperations":null}`, "", "channel_header_override_unsupported"},
		{"environment ignores stale URL", `{"proxy":{"type":"environment","url":"not-a-url"}}`, "", ""},
		{"disabled proxy cannot silently use stale URL", `{"proxy":{"type":"disabled","url":"https://proxy.invalid"}}`, "", "proxy_configuration_unsupported"},
		{"connection reuse contract", `{"proxy":{"type":"url","url":"https://proxy.invalid","disableConnectionReuse":true}}`, "", "channel_settings_unsupported:proxy.disableConnectionReuse"},
		{"unknown proxy field", `{"proxy":{"type":"url","url":"https://proxy.invalid","futureField":true}}`, "", "channel_settings_unsupported:proxy.futureField"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, err := ParseAxonHubSource(axonHubSettingsPayload(tc.settings))
			if err != nil {
				t.Fatal(err)
			}
			plan, err := CompileAxonHubPlan(src)
			if err != nil {
				t.Fatal(err)
			}
			if tc.blocked != "" {
				if len(plan.skipped) != 1 || !residualContains(plan.skipped[0].Reasons, tc.blocked) {
					t.Fatalf("missing execution guard %q: %#v", tc.blocked, plan.skipped)
				}
				return
			}
			if len(plan.channels) != 1 {
				t.Fatalf("valid settings were refused: %#v", plan.skipped)
			}
			if got := plan.channels[0].ParamOverride; got != tc.override {
				t.Fatalf("effective parameters = %q, want %q", got, tc.override)
			}
		})
	}
}

func TestAxonHubProxyCredentialsPreservedWithoutPreviewLeak(t *testing.T) {
	raw := axonHubSettingsPayload(`{"proxy":{"type":"url","url":"https://proxy.invalid:8080","username":"fixture-user","password":"fixture-password"}}`)
	src, err := ParseAxonHubSource(raw)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil {
		t.Fatal(err)
	}
	wantProxy := (&url.URL{Scheme: "https", Host: "proxy.invalid:8080", User: url.UserPassword("fixture-user", "fixture-password")}).String()
	if len(plan.channels) != 1 || plan.channels[0].ChannelProxy != wantProxy {
		t.Fatal("separate proxy credentials were not retained")
	}
	preview, err := PreviewAxonHubV14(nil, raw, "proxy-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprintf("%+v", preview), "fixture-password") {
		t.Fatal("proxy password leaked in preview")
	}
}
