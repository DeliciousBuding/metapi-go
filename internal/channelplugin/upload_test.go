package channelplugin

import (
	"strings"
	"testing"
)

const uploadSource = `export const meta={apiVersion:2,kind:"provider",key:"upload",name:"Upload",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],requestKinds:["json","multipart"],hooks:["submit","query","content","taskResult"]};export function submit(i){if(i.bodyKind==="multipart"&&Object.keys(i.files[0]).sort().join(",")!=="field,filename,mimeType,ref,size")throw new Error("unexpected file capability");return {method:"POST",body:i.body};}export function query(){return {method:"GET"};}export function content(){return {method:"GET"};}export function taskResult(i){return {body:i.body};}`

func TestProviderRequestKindsAndFileMetadata(t *testing.T) {
	program, err := Compile(t.Context(), uploadSource)
	if err != nil {
		t.Fatal(err)
	}
	input := DriverInput{Protocol: "video", CredentialKind: "api_key", BodyKind: "multipart", Body: `{"model":"fixture"}`, Files: []FileReference{{Ref: "fixture-ref", Field: "input_reference", Filename: "image.png", MIMEType: "image/png", Size: 4}}}
	if _, err := program.CallDriver(t.Context(), "submit", input); err != nil {
		t.Fatal(err)
	}
	copy := program.Manifest()
	copy.RequestKinds[1] = "form"
	if !program.Manifest().AcceptsRequestKind("multipart") {
		t.Fatal("request form snapshot mutated")
	}
	legacy, err := Compile(t.Context(), strings.Replace(uploadSource, `requestKinds:["json","multipart"],`, "", 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.CallDriver(t.Context(), "submit", input); err != ErrRequestKind {
		t.Fatalf("legacy provider accepted files: %v", err)
	}
	for _, declaration := range []string{`requestKinds:[],`, `requestKinds:["multipart","multipart"],`, `requestKinds:["form"],`} {
		if _, err := Compile(t.Context(), strings.Replace(uploadSource, `requestKinds:["json","multipart"],`, declaration, 1)); err != ErrManifest {
			t.Fatalf("invalid form contract accepted: %v", err)
		}
	}
	input.Files = append(input.Files, input.Files[0])
	if _, err := program.CallDriver(t.Context(), "submit", input); err != ErrResult {
		t.Fatalf("duplicate refs accepted: %v", err)
	}
}

func TestMultipartDescriptorIsTypedAndHasNoNetworkOrBoundaryAuthority(t *testing.T) {
	value := "fixture"
	valid := RequestDescriptor{Method: "POST", Parts: []MultipartPart{{Field: "model", Value: &value}, {Field: "image", FileRef: "fixture-ref"}}}
	if valid.Validate() != nil {
		t.Fatal("valid multipart descriptor rejected")
	}
	for _, descriptor := range []RequestDescriptor{
		{Method: "GET", Parts: valid.Parts},
		{Method: "POST", Body: `{}`, Parts: valid.Parts},
		{Method: "POST", ContentType: "multipart/form-data; boundary=owned", Parts: valid.Parts},
		{Method: "POST", Parts: []MultipartPart{}},
		{Method: "POST", Parts: []MultipartPart{{Field: "image", Value: &value, FileRef: "fixture-ref"}}},
		{Method: "POST", Parts: []MultipartPart{{Field: "image"}}},
		{Method: "POST", Parts: valid.Parts, Headers: map[string]string{"Content-Type": "multipart/form-data"}},
		{Method: "POST", Parts: valid.Parts, Headers: map[string]string{"Authorization": "forbidden"}},
	} {
		if descriptor.Validate() != ErrResult {
			t.Fatalf("unsafe descriptor accepted: %+v", descriptor)
		}
	}
}
