package channelplugins_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

const imageQuerySource = `export const meta={apiVersion:2,kind:"provider",key:"image-task",name:"Image task",version:"1.0.0",protocols:["imageGeneration"],credentialKinds:["api_key"],hooks:["submit","query","taskResult"],observations:{imageGeneration:{response:{taskIdPath:"/task_id"}}},imageTasks:{imageGeneration:{query:{path:"/tasks/{taskId}",auth:"bearer"}}}};export function submit(i){return {method:"POST",body:i.body};}export function query(){return {method:"GET"};}export function taskResult(){return {status:"pending"};}`

func TestImageQueryDefaultsRemainExplicitAndDoNotFollowProgramUpgrade(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := database(t, dialect)
			if _, err := channelplugins.Save(t.Context(), db.DB, "image-task", imageQuerySource, true, 0); err != nil {
				t.Fatal(err)
			}
			binding := store.DirectProviderBinding{Key: "image-task", Protocols: []string{"imageGeneration"}}
			var endpoints store.DirectEndpoints
			if endpoints.Scan(`{"imageGeneration":{"url":"https://selected.example/custom/images/generations","auth":"bearer"}}`) != nil {
				t.Fatal("invalid fixture endpoint")
			}
			manager := channelplugins.New(db.DB)
			if _, err := manager.ResolveDriver(t.Context(), binding, "imageGeneration", "api_key"); err == nil {
				t.Fatal("missing query target did not block dispatch")
			}
			config := store.DirectRequestConfig{Provider: &binding}
			if err := channelplugins.FillDefaults(t.Context(), db.DB, "https://unselected.example/base", &endpoints, &config); err != nil {
				t.Fatal(err)
			}
			query := config.Provider.ImageQueries["imageGeneration"]
			if query.URL != "https://selected.example/tasks/{taskId}" || query.Auth != "bearer" || len(binding.ImageQueries) != 0 {
				t.Fatalf("defaults changed source binding or used unselected origin: %+v", query)
			}
			if err := channelplugins.ValidateBindings(t.Context(), db.DB, config, endpoints); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.ResolveDriver(t.Context(), *config.Provider, "imageGeneration", "api_key"); err != nil {
				t.Fatal(err)
			}
			// The saved operation is independent of future template defaults.
			if _, err := channelplugins.Save(t.Context(), db.DB, "image-task", strings.Replace(imageQuerySource, `/tasks/{taskId}`, `/changed/{taskId}`, 1), true, 1); err != nil {
				t.Fatal(err)
			}
			if channelplugins.FillDefaults(t.Context(), db.DB, "https://unselected.example/base", &endpoints, &config) != nil || config.Provider.ImageQueries["imageGeneration"] != query {
				t.Fatal("program upgrade moved the saved query target")
			}
			encoded, err := json.Marshal(config)
			var restored store.DirectRequestConfig
			if err != nil || restored.Scan(encoded) != nil || restored.Provider.ImageQueries["imageGeneration"] != query {
				t.Fatal("query target did not survive storage round trip")
			}
		})
	}
}
