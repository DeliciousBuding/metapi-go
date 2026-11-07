package channelplugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
)

const meta = `export const meta={apiVersion:1,key:"fixture",name:"Fixture",version:"1.0.0",protocols:["chat"],hooks:["request"]};`

const providerMeta = `export const meta={apiVersion:2,kind:"provider",key:"fixture-provider",name:"Fixture provider",version:"1.0.0",protocols:["chat","video"],credentialKinds:["api_key","oauth"],hooks:["request","response","modelsRequest","modelsResponse","submit","query","content","taskResult"]};`

const providerFunctions = `export function request(input){return {method:"POST",path:"/generate",body:input.body};}
export function response(input){return {body:input.body};}
export function modelsRequest(){return {method:"GET",path:"/models"};}
export function modelsResponse(){return {models:["vendor-model"]};}
export function submit(input){return {method:"POST",path:"/jobs",body:input.body};}
export function query(input){return {method:"GET",path:"/jobs/"+encodeURIComponent(input.taskId)};}
export function content(){return {method:"GET",path:"/content"};}
export function taskResult(){return {status:"pending"};}`

func TestProviderDeclarationIsIndependentFromWireHooks(t *testing.T) {
	p, err := Compile(context.Background(), providerMeta+providerFunctions)
	if err != nil {
		t.Fatalf("valid provider declaration cannot be installed: %v", err)
	}
	// A provider returns descriptors, not wire operations. Existing wire-hook
	// bindings must not interpret one as a no-op and silently skip its driver.
	if _, err := p.Call(context.Background(), "request", Input{Protocol: "chat", Body: `{}`}); !errors.Is(err, ErrManifest) {
		t.Fatalf("provider borrowed the wire-hook entry: %v", err)
	}
}

func TestProviderCallsKeepNativeJSONAndSeparateDiscovery(t *testing.T) {
	p, err := Compile(context.Background(), providerMeta+providerFunctions)
	if err != nil {
		t.Fatal(err)
	}
	wire := `{"model":"vendor-model","seed":9007199254740993,"opaque":"untouched"}`
	raw, err := p.CallDriver(context.Background(), "request", DriverInput{Protocol: "chat", Body: wire, CredentialKind: "api_key"})
	var descriptor RequestDescriptor
	if err != nil || json.Unmarshal(raw, &descriptor) != nil || descriptor.Body != wire {
		t.Fatalf("driver changed native input: %s %v", raw, err)
	}
	endpoint, err := descriptor.ResolveURL("https://provider.invalid/v1/chat")
	if err != nil || endpoint.String() != "https://provider.invalid/generate" {
		t.Fatalf("descriptor changed destination: %v %v", endpoint, err)
	}
	raw, err = p.CallDriver(context.Background(), "modelsResponse", DriverInput{CredentialKind: "oauth"})
	if err != nil || string(raw) != `{"models":["vendor-model"]}` {
		t.Fatalf("discovery callback failed: %s %v", raw, err)
	}
	for _, input := range []DriverInput{
		{Protocol: "responses", CredentialKind: "api_key"},
		{Protocol: "chat", CredentialKind: "none"},
		{Protocol: "chat", CredentialKind: "api_key", Stream: true},
	} {
		if _, err := p.CallDriver(context.Background(), "request", input); !errors.Is(err, ErrManifest) {
			t.Fatalf("undeclared driver capability accepted: %+v %v", input, err)
		}
	}
	if _, err := p.CallDriver(context.Background(), "modelsResponse", DriverInput{Protocol: "chat", CredentialKind: "oauth"}); !errors.Is(err, ErrManifest) {
		t.Fatalf("discovery borrowed a transport slot: %v", err)
	}
	for _, hook := range []string{"submit", "query", "content", "taskResult"} {
		if _, err := p.CallDriver(context.Background(), hook, DriverInput{Protocol: "video", CredentialKind: "api_key", TaskID: "job/1"}); err != nil {
			t.Fatalf("declared task operation %s failed: %v", hook, err)
		}
	}
}

func TestProviderDescriptorsRejectRecipientAndHeaderInjection(t *testing.T) {
	for _, descriptor := range []RequestDescriptor{
		{Method: "POST", Path: "https://other.invalid/jobs"},
		{Method: "POST", Path: "//other.invalid/jobs"},
		{Method: "POST", Path: "/jobs#fragment"},
		{Method: "POST", Path: "/jobs\\evil"},
		{Method: "TRACE", Path: "/jobs"},
		{Method: "GET", Body: "not-read-only"},
		{Method: "POST", Headers: map[string]string{"Authorization": "synthetic-private"}},
		{Method: "POST", Headers: map[string]string{"X-Vendor-Token": "synthetic-private"}},
		{Method: "POST", Headers: map[string]string{"Transfer-Encoding": "chunked"}},
		{Method: "POST", Headers: map[string]string{"X-Vendor": "value\r\nInjected: header"}},
		{Method: "POST", Headers: map[string]string{"X-Vendor": "one", "x-vendor": "two"}},
	} {
		if _, err := descriptor.ResolveURL("https://provider.invalid/v1"); !errors.Is(err, ErrResult) {
			t.Fatalf("unsafe descriptor accepted: %+v %v", descriptor, err)
		}
	}
}

func TestProviderCallsIsolateStateAndBoundErrors(t *testing.T) {
	declaration := `export const meta={apiVersion:2,kind:"provider",key:"fixture",name:"F",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response"]};export function response(input){return {body:input.body};}`
	for _, hook := range []string{
		`export function request(){throw new Error("synthetic-private-input");}`,
		`export async function request(){return {};}`,
		`export function request(){return [];}`,
	} {
		p, err := Compile(context.Background(), declaration+hook)
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.CallDriver(context.Background(), "request", DriverInput{Protocol: "chat", CredentialKind: "api_key"})
		if err == nil || strings.Contains(err.Error(), "synthetic-private-input") {
			t.Fatalf("unsafe provider error/result: %v", err)
		}
	}
	p, err := Compile(context.Background(), declaration+`let count=0;export function request(){if(typeof fetch!=="undefined"||typeof process!=="undefined")throw new Error("ambient");return {count:++count};}`)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		raw, err := p.CallDriver(context.Background(), "request", DriverInput{Protocol: "chat", CredentialKind: "api_key"})
		if err != nil || string(raw) != `{"count":1}` {
			t.Fatalf("provider retained ambient state: %s %v", raw, err)
		}
	}
}

func TestActualModuleOperationsPreserveIntegerAndNativeExtensions(t *testing.T) {
	p, err := Compile(context.Background(), meta+`export function request(input){return {body:[{op:"set",path:"vendor.flag",value:"true"}],headers:[{op:"set",path:"X-Vendor-Mode",value:"native"}]};}`)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"model":"m","stream":true,"seed":9007199254740993,"tools":[{"vendor":{"x":18446744073709551615}}]}`)
	result, err := p.Call(context.Background(), "request", Input{Protocol: "chat", Body: string(raw)})
	if err != nil {
		t.Fatal(err)
	}
	out, warnings, err := requestoverride.ApplyBody(raw, result.Body, requestoverride.Context{}, MaxInputBytes)
	if err != nil || len(warnings) != 0 || !strings.Contains(string(out), `9007199254740993`) || !strings.Contains(string(out), `18446744073709551615`) {
		t.Fatalf("wire lost: %s %v %v", out, warnings, err)
	}
}

func TestManifestAndResultFailClosed(t *testing.T) {
	for _, source := range []string{
		`import x from "fs";` + meta,
		`export const meta={apiVersion:2,key:"fixture",name:"F",version:"1.0.0",protocols:["chat"],hooks:["request"]};export function request(){return {};}`,
		meta,
		meta + `export function request(){return null;}`,
		meta + `export function request(){return {unknown:1};}`,
		meta + `export function request(){return {headers:[{op:"set",path:"Authorization",value:"fixture"}]};}`,
		meta + `export function request(){return {body:[{op:"set",path:"model",value:"unauthorized"}]};}`,
		meta + `export function request(){return {body:[{op:"set",path:"max_tokens",value:"999999"}]};}`,
		meta + `export async function request(){return {};}`,
		meta + `export function request(){return {get body(){throw new Error("private-input");}};}`,
	} {
		t.Run(source[len(source)/2:], func(t *testing.T) {
			p, err := Compile(context.Background(), source)
			if err == nil {
				_, err = p.Call(context.Background(), "request", Input{Protocol: "chat", Body: `{}`})
			}
			if err == nil || strings.Contains(err.Error(), "private-input") {
				t.Fatalf("unsafe success/error: %v", err)
			}
		})
	}
}

func TestRuntimeHasNoAmbientCapabilitiesAndNoCrossCallState(t *testing.T) {
	p, err := Compile(context.Background(), meta+`let count=0;export function request(){if(typeof fetch!=="undefined"||typeof process!=="undefined"||typeof require!=="undefined")throw new Error("ambient");count++;return {body:[{op:"set",path:"count",value:String(count)}]};}`)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := p.Call(context.Background(), "request", Input{Protocol: "chat", Body: `{}`})
			if e != nil || len(r.Body) != 1 || r.Body[0].Value != "1" {
				t.Errorf("state leak: %+v %v", r, e)
			}
		}()
	}
	wg.Wait()
}

func TestInterruptIncludesModuleInitializationAndSerialization(t *testing.T) {
	for _, source := range []string{
		meta + `while(true){};export function request(){return {};}`,
		meta + `export function request(){while(true){}}`,
		meta + `export function request(){return {toJSON(){while(true){}}};}`,
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		started := time.Now()
		p, err := Compile(ctx, source)
		if err == nil {
			_, err = p.Call(ctx, "request", Input{Protocol: "chat", Body: `{}`})
		}
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
			t.Fatalf("not interrupted: %v", err)
		}
	}
}

func TestOptionsDefaultsValidationAndManifestCopies(t *testing.T) {
	source := `export const meta={apiVersion:1,key:"fixture",name:"Fixture",version:"1.0.0",protocols:["chat"],hooks:["request"],options:[{key:"mode",label:"Mode",type:"select",default:"safe",choices:["safe","fast"]}]};export function request(input){return {body:[{op:"set",path:"vendor.mode",value:input.options.mode}]};}`
	p, err := Compile(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	m := p.Manifest()
	m.Protocols[0] = "unknown"
	m.Options[0].Default = json.RawMessage(`"wrong"`)
	r, err := p.Call(context.Background(), "request", Input{Protocol: "chat", Body: `{}`})
	if err != nil || r.Body[0].Value != "safe" {
		t.Fatalf("default changed: %+v %v", r, err)
	}
	for _, options := range []map[string]json.RawMessage{{"missing": json.RawMessage(`true`)}, {"mode": json.RawMessage(`"wrong"`)}, {"mode": json.RawMessage(`null`)}} {
		if _, err := p.Call(context.Background(), "request", Input{Protocol: "chat", Body: `{}`, Options: options}); err == nil {
			t.Fatal("invalid options accepted")
		}
	}
}
