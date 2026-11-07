package channelplugin

import (
	"errors"
	"strings"
	"testing"
)

func TestProviderAuthenticationCompileAndImmutableMetadata(t *testing.T) {
	const source = `export const meta={apiVersion:2,kind:"provider",key:"auth",name:"Auth",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response"],authentication:{type:"api_key",header:"X-Vendor-Key",prefix:"Key "}};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}`
	p, err := Compile(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	metadata := p.Manifest()
	metadata.Authentication.Header = "Host"
	if p.Manifest().Authentication.Header != "X-Vendor-Key" {
		t.Fatal("caller mutated captured authentication")
	}
	for _, bad := range []string{
		strings.Replace(source, `"X-Vendor-Key"`, `"Host"`, 1),
		strings.Replace(source, `credentialKinds:["api_key"]`, `credentialKinds:["api_key","oauth"]`, 1),
		strings.Replace(source, `type:"api_key"`, `type:"oauth2_jwt"`, 1),
		strings.Replace(source, `prefix:"Key "`, `prefix:"Key ",secret:"synthetic-private-value"`, 1),
		strings.Replace(source, `apiVersion:2,kind:"provider"`, `apiVersion:1`, 1),
	} {
		if _, err := Compile(t.Context(), bad); !errors.Is(err, ErrManifest) || strings.Contains(err.Error(), "synthetic-private-value") {
			t.Fatal("invalid authentication manifest accepted or exposed source")
		}
	}
}
