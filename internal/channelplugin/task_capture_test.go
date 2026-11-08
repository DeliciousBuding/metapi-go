package channelplugin

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestDurableTaskCaptureMetadataAndLegacyFingerprint(t *testing.T) {
	const source = `export const meta={apiVersion:2,kind:"provider",key:"capture",name:"Capture",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],hooks:["submit","query","content","taskResult"]};export function submit(i){return {method:"POST",body:i.body};}export function query(){return {method:"GET"};}export function content(){return {method:"GET"};}export function taskResult(i){return {body:i.body};}`
	const metadata = `{"apiVersion":2,"kind":"provider","key":"capture","name":"Capture","version":"1.0.0","protocols":["video"],"hooks":["submit","query","content","taskResult"],"credentialKinds":["api_key"]}`
	legacy, err := Compile(t.Context(), source)
	if err != nil || legacy.Fingerprint() != fmt.Sprintf("%x", sha256.Sum256([]byte(source+"\x00"+metadata))) {
		t.Fatalf("legacy executable fingerprint changed: %v", err)
	}
	durable := strings.Replace(source, `apiVersion:2,`, `apiVersion:2,taskCapture:"durable",`, 1)
	program, err := Compile(t.Context(), durable)
	if err != nil || !program.Manifest().DurableTaskCapture() || program.Fingerprint() == legacy.Fingerprint() {
		t.Fatalf("durable contract not captured: %v", err)
	}
	copy := program.Manifest()
	*copy.TaskCapture = "changed"
	if !program.Manifest().DurableTaskCapture() {
		t.Fatal("metadata copy changed the executable contract")
	}
	for _, invalid := range []string{
		strings.Replace(durable, `taskCapture:"durable"`, `taskCapture:"legacy"`, 1),
		strings.Replace(durable, `taskCapture:"durable"`, `taskCapture:""`, 1),
		strings.Replace(durable, `taskCapture:"durable"`, `taskCapture:true`, 1),
		strings.Replace(durable, `protocols:["video"]`, `protocols:["chat"]`, 1),
	} {
		if _, err := Compile(t.Context(), invalid); err == nil {
			t.Fatal("invalid capture contract accepted")
		}
	}
}
