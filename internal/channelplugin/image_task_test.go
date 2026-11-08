package channelplugin

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const imageTaskSource = `export const meta={apiVersion:2,kind:"provider",key:"image-task",name:"Image task",version:"1.0.0",protocols:["modelscopeImageGeneration"],credentialKinds:["api_key"],hooks:["submit","query","taskResult"],observations:{modelscopeImageGeneration:{response:{taskIdPath:"/task_id"}}},imageTasks:{modelscopeImageGeneration:{query:{path:"/vendor/tasks/{taskId}",auth:"bearer"}}}};export function submit(i){return {method:"POST",body:i.body};}export function query(){return {method:"GET"};}export function taskResult(i){return {status:"pending"};}`

func TestImageTaskStreamCoexistsWithAuthenticationAndDurableCapture(t *testing.T) {
	source := strings.Replace(imageTaskSource, `apiVersion:2,`, `apiVersion:2,taskCapture:"durable",authentication:{type:"api_key",header:"X-Vendor-Key",prefix:"Key "},`, 1)
	source = strings.Replace(source, `protocols:["modelscopeImageGeneration"]`, `protocols:["modelscopeImageGeneration","video"]`, 1)
	source = strings.Replace(source, `hooks:["submit","query","taskResult"]`, `hooks:["submit","query","content","taskResult"]`, 1)
	source = strings.Replace(source, `query:{path`, `stream:true,query:{path`, 1) + `export function content(){return {method:"GET"};}`
	program, err := Compile(t.Context(), source)
	if err != nil {
		t.Fatalf("combined host capabilities rejected: %v", err)
	}
	manifest := program.Manifest()
	if !manifest.DurableTaskCapture() || manifest.Authentication == nil || manifest.Authentication.Header != "X-Vendor-Key" || !program.StreamsImageTask("modelscopeImageGeneration") || program.StreamsImageTask("video") {
		t.Fatal("combined declaration lost or widened a capability")
	}
	for _, hook := range []string{"submit", "query", "taskResult"} {
		if _, err = program.CallDriver(t.Context(), hook, DriverInput{Protocol: "modelscopeImageGeneration", CredentialKind: "api_key", Body: `{}`, Stream: true}); err != nil {
			t.Fatalf("image stream invocation with capture/authentication failed: %v", err)
		}
	}
	*manifest.TaskCapture = "changed"
	manifest.Authentication.Header = "X-Changed"
	task := manifest.ImageTasks["modelscopeImageGeneration"]
	task.Stream = false
	manifest.ImageTasks["modelscopeImageGeneration"] = task
	if !program.Manifest().DurableTaskCapture() || program.Manifest().Authentication.Header != "X-Vendor-Key" || !program.StreamsImageTask("modelscopeImageGeneration") {
		t.Fatal("manifest copy mutated captured capabilities")
	}
	for _, invalid := range []string{
		strings.Replace(source, `taskCapture:"durable"`, `taskCapture:"legacy"`, 1),
		strings.Replace(source, `"modelscopeImageGeneration","video"`, `"modelscopeImageGeneration"`, 1),
		strings.Replace(source, `credentialKinds:["api_key"]`, `credentialKinds:["oauth"]`, 1),
		strings.ReplaceAll(source, "modelscopeImageGeneration", "imageVariation"),
	} {
		if _, err = Compile(t.Context(), invalid); err == nil {
			t.Fatal("combining capabilities relaxed an independent validation")
		}
	}
}

func TestImageTaskDeclarationRequiresExplicitProtocolAndQueryTemplate(t *testing.T) {
	program, err := Compile(t.Context(), imageTaskSource)
	if err != nil {
		t.Fatalf("explicit asynchronous image lifecycle was rejected: %v", err)
	}
	raw, _ := json.Marshal(program.Manifest())
	if !strings.Contains(string(raw), `"imageTasks":{"modelscopeImageGeneration":{"query":{"path":"/vendor/tasks/{taskId}","auth":"bearer"}}}`) {
		t.Fatalf("image task declaration was not retained: %s", raw)
	}
	copy := program.Manifest()
	task := copy.ImageTasks["modelscopeImageGeneration"]
	task.Query.Path = "/changed/{taskId}"
	copy.ImageTasks["modelscopeImageGeneration"] = task
	if program.Manifest().ImageTasks["modelscopeImageGeneration"].Query.Path != "/vendor/tasks/{taskId}" {
		t.Fatal("a mutable manifest copy changed the captured operation template")
	}
	for _, invalid := range []string{
		strings.Replace(imageTaskSource, `"submit","query","taskResult"`, `"submit","taskResult"`, 1),
		strings.ReplaceAll(imageTaskSource, `modelscopeImageGeneration`, `chat`),
		strings.Replace(imageTaskSource, `/vendor/tasks/{taskId}`, `/vendor/tasks`, 1),
		strings.Replace(imageTaskSource, `/vendor/tasks/{taskId}`, `/vendor/{taskId}/{taskId}`, 1),
		strings.Replace(imageTaskSource, `/vendor/tasks/{taskId}`, `/vendor/{taskId}suffix`, 1),
		strings.Replace(imageTaskSource, `/vendor/tasks/{taskId}`, `//other.example/{taskId}`, 1),
		strings.Replace(imageTaskSource, `/vendor/tasks/{taskId}`, `/vendor/%2e%2e/{taskId}`, 1),
		strings.Replace(imageTaskSource, `/vendor/tasks/{taskId}`, `/vendor/tasks/{taskId}?token=fixture`, 1),
		strings.Replace(imageTaskSource, `auth:"bearer"`, `auth:"cookie"`, 1),
		strings.Replace(imageTaskSource, `taskIdPath:"/task_id"`, `modelPath:"/model"`, 1),
		strings.Replace(imageTaskSource, `apiVersion:2,kind:"provider"`, `apiVersion:1`, 1),
	} {
		if _, err := Compile(t.Context(), invalid); err == nil {
			t.Fatal("invalid or unbound image query declaration was accepted")
		}
	}
}

func TestImageTaskExtensionDoesNotRewriteSynchronousManifestIdentity(t *testing.T) {
	const source = `export const meta={apiVersion:2,kind:"provider",key:"image-task",name:"Image task",version:"1.0.0",protocols:["imageGeneration"],credentialKinds:["api_key"],hooks:["request","response"]};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}`
	const metadata = `{"apiVersion":2,"kind":"provider","key":"image-task","name":"Image task","version":"1.0.0","protocols":["imageGeneration"],"hooks":["request","response"],"credentialKinds":["api_key"]}`
	program, err := Compile(t.Context(), source)
	if err != nil || program.Fingerprint() != fmt.Sprintf("%x", sha256.Sum256([]byte(source+"\x00"+metadata))) {
		t.Fatalf("legacy synchronous program identity changed: %v", err)
	}
}

func TestImageTaskStreamIsExplicitAndDoesNotRequireTextEventHooks(t *testing.T) {
	for _, protocol := range []string{"imageGeneration", "imageEdit", "modelscopeImageGeneration"} {
		source := strings.ReplaceAll(imageTaskSource, "modelscopeImageGeneration", protocol)
		legacy, err := Compile(t.Context(), source)
		if err != nil {
			t.Fatal(err)
		}
		if legacy.StreamsImageTask(protocol) {
			t.Fatal("legacy declaration gained streaming")
		}
		_, err = legacy.CallDriver(t.Context(), "submit", DriverInput{Protocol: protocol, CredentialKind: "api_key", Body: `{}`, Stream: true})
		if err == nil {
			t.Fatal("undeclared streaming execution accepted")
		}
		source = strings.Replace(source, `query:{path`, `stream:true,query:{path`, 1)
		program, err := Compile(t.Context(), source)
		if err != nil || !program.StreamsImageTask(protocol) {
			t.Fatalf("explicit image stream rejected: %v", err)
		}
		for _, hook := range []string{"submit", "query", "taskResult"} {
			if _, err = program.CallDriver(t.Context(), hook, DriverInput{Protocol: protocol, CredentialKind: "api_key", Body: `{}`, Stream: true}); err != nil {
				t.Fatalf("image streaming required unrelated event hook: %v", err)
			}
		}
		if program.Fingerprint() == legacy.Fingerprint() {
			t.Fatal("stream capability did not change executable identity")
		}
		if _, err = Compile(t.Context(), strings.ReplaceAll(source, protocol, "imageVariation")); err == nil {
			t.Fatal("variation inherited generation streaming")
		}
	}
}
