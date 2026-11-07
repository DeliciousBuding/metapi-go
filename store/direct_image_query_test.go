package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProviderImageQueryTargetIsSavedAndBoundToItsExactSlotOrigin(t *testing.T) {
	const raw = `{"provider":{"key":"image-task","protocols":["imageGeneration"],"imageQueries":{"imageGeneration":{"url":"https://vendor.example/tasks/{taskId}","auth":"bearer"}}}}`
	var config DirectRequestConfig
	if err := config.Scan(raw); err != nil {
		t.Fatal(err)
	}
	var endpoints DirectEndpoints
	if endpoints.Scan(`{"imageGeneration":{"url":"https://vendor.example/images/generations","auth":"bearer"}}`) != nil || config.ValidateEndpoints(endpoints) != nil {
		t.Fatal("query target matching its selected image origin was rejected")
	}
	encoded, _ := json.Marshal(config)
	var restored DirectRequestConfig
	if restored.Scan(encoded) != nil || restored.Provider.ImageQueries["imageGeneration"].URL != "https://vendor.example/tasks/{taskId}" {
		t.Fatal("query target lost in request configuration")
	}
	for _, invalid := range []string{
		strings.Replace(raw, "vendor.example/tasks", "other.example/tasks", 1),
		strings.Replace(raw, "vendor.example/tasks", "vendor.example:444/tasks", 1),
		strings.Replace(raw, `https://vendor.example/tasks`, `http://vendor.example/tasks`, 1),
		strings.Replace(raw, `"imageQueries":{"imageGeneration"`, `"imageQueries":{"imageEdit"`, 1),
		strings.Replace(raw, `/tasks/{taskId}`, `/tasks/{taskId}?token=fixture`, 1),
		strings.Replace(raw, `/tasks/{taskId}`, `/tasks/static`, 1),
		strings.Replace(raw, `"auth":"bearer"`, `"auth":"cookie"`, 1),
	} {
		var bad DirectRequestConfig
		if bad.Scan(invalid) == nil && bad.ValidateEndpoints(endpoints) == nil {
			t.Fatalf("unbound image operation accepted: %s", invalid)
		}
	}
}
