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
