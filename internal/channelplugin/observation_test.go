package channelplugin

import (
	"strings"
	"testing"
)

const observationSource = `export const meta={apiVersion:2,kind:"provider",key:"observer",name:"Observer",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response","event","decodeResponse"],observations:{chat:{response:{usagePaths:{prompt_tokens:"/a.b/a~1b/~0count",completion_tokens:"/array/0/count",total_tokens:"/total"},modelPath:"/engine",errorPath:"/failure"}}}};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}export function event(i){return {body:i.body};}export function decodeResponse(){throw new Error("private decoder");}`

func TestProviderObservationsAreLiteralAndIndependentFromExecution(t *testing.T) {
	program, err := Compile(t.Context(), observationSource)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"a.b":{"a/b":{"~count":9007199254740993}},"array":[{"count":0}],"total":null,"engine":"vendor-model","failure":{"message":"private detail"}}`)
	observed, err := program.Observe("chat", false, body)
	if err != nil || !strings.Contains(string(observed.Usage), "9007199254740993") || !strings.Contains(string(observed.Usage), `"completion_tokens":0`) || !strings.Contains(string(observed.Usage), `"total_tokens":null`) || string(observed.Model) != `"vendor-model"` || !observed.Failed {
		t.Fatalf("original facts changed: %+v %v", observed, err)
	}
	if _, err := program.CallDriver(t.Context(), "decodeResponse", DriverInput{Protocol: "chat", CredentialKind: "api_key", Body: string(body)}); err != ErrExecution {
		t.Fatalf("throwing decoder did not fail: %v", err)
	}
	manifest := program.Manifest()
	manifest.Observations["chat"].Response.UsagePaths["prompt_tokens"] = "/other"
	again, err := program.Observe("chat", false, body)
	if err != nil || string(again.Usage) != string(observed.Usage) {
		t.Fatalf("manifest copy mutated pinned observations: %s %v", again.Usage, err)
	}
	missing, err := program.Observe("chat", false, []byte(`{"array":[]}`))
	if err != nil || string(missing.Usage) != `{"usage":{}}` || missing.Model != nil || missing.Failed {
		t.Fatalf("missing partial facts became data: %+v %v", missing, err)
	}
	if _, err := program.Observe("chat", false, []byte(`{"total":1,"total":9}`)); err != ErrResult {
		t.Fatalf("ambiguous source was accepted: %v", err)
	}
}

func TestProviderObservationManifestRejectsUnknownOrExecutableSelectors(t *testing.T) {
	for _, source := range []string{
		strings.Replace(observationSource, `"/total"`, `"#total"`, 1),
		strings.Replace(observationSource, `"/total"`, `"/a~2b"`, 1),
		strings.Replace(observationSource, `total_tokens:`, `invoice:`, 1),
		strings.Replace(observationSource, `observations:{chat:`, `observations:{responses:`, 1),
		strings.Replace(observationSource, `modelPath:"/engine"`, `modelPath:"/bad\npath"`, 1),
	} {
		if _, err := Compile(t.Context(), source); err != ErrManifest {
			t.Fatalf("invalid observation declaration accepted: %v", err)
		}
	}
	legacy := `export const meta={apiVersion:1,key:"observer",name:"Observer",version:"1.0.0",protocols:["chat"],hooks:["request"],observations:{}};export function request(){return {};}`
	if _, err := Compile(t.Context(), legacy); err != ErrManifest {
		t.Fatalf("wire hooks acquired provider observation authority: %v", err)
	}
}
