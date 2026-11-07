package channelplugin

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const imageTaskSource = `export const meta={apiVersion:2,kind:"provider",key:"image-task",name:"Image task",version:"1.0.0",protocols:["modelscopeImageGeneration"],credentialKinds:["api_key"],hooks:["submit","query","taskResult"],observations:{modelscopeImageGeneration:{response:{taskIdPath:"/task_id"}}},imageTasks:{modelscopeImageGeneration:{query:{path:"/vendor/tasks/{taskId}",auth:"bearer"}}}};export function submit(i){return {method:"POST",body:i.body};}export function query(){return {method:"GET"};}export function taskResult(i){return {status:"pending"};}`

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
