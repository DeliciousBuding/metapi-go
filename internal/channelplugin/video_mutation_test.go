package channelplugin

import (
	"strings"
	"testing"
)

func TestProviderVideoMutationsAreExplicitAndVideoOnly(t *testing.T) {
	source := strings.Replace(providerMeta, `"taskResult"]`, `"taskResult","deleteTask","remix"]`, 1) + providerFunctions + `export function deleteTask(i){return {method:"DELETE"};}export function remix(i){return {method:"POST",body:i.body};}`
	p, err := Compile(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	for _, hook := range []string{"deleteTask", "remix"} {
		input := DriverInput{Protocol: "video", CredentialKind: "api_key", TaskID: "job/ ?#%"}
		if hook == "remix" {
			input.Body = `{"prompt":"change","seed":9007199254740993}`
		}
		if _, err := p.CallDriver(t.Context(), hook, input); err != nil {
			t.Fatalf("explicit %s rejected: %v", hook, err)
		}
		for _, invalid := range []DriverInput{
			{Protocol: "chat", CredentialKind: "api_key", TaskID: input.TaskID},
			{Protocol: "video", CredentialKind: "api_key", TaskID: input.TaskID, Stream: true},
			{Protocol: "video", CredentialKind: "api_key"},
			{Protocol: "video", CredentialKind: "api_key", TaskID: input.TaskID, BodyKind: "multipart"},
		} {
			if _, err := p.CallDriver(t.Context(), hook, invalid); err == nil {
				t.Fatalf("invalid %s input accepted: %+v", hook, invalid)
			}
		}
	}
	for _, hook := range []string{"deleteTask", "remix"} {
		textOnly := `export const meta={apiVersion:2,kind:"provider",key:"text",name:"Text",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response","` + hook + `"]};` + providerFunctions + `export function ` + hook + `(){return {};}`
		if _, err := Compile(t.Context(), textOnly); err == nil {
			t.Fatalf("text-only provider declared %s", hook)
		}
	}
}
