package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

const testModule = `export const meta={apiVersion:2,kind:"provider",key:"maintain",name:"Maintain",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],hooks:["submit","query","content","taskResult"]};export function submit(i){return {method:"POST",body:i.body};}export function query(){return {method:"GET"};}export function content(){return {method:"GET"};}export function taskResult(i){return {body:i.body};}`

func TestCLIInspectCheckApplyAndPrivateOutput(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hub.db")
	db, err := store.Open(store.DialectSQLite, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	program, err := channelplugin.Compile(t.Context(), testModule)
	if err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(program.Manifest())
	if _, err = db.Exec(`INSERT INTO channel_plugin_versions(plugin_key,fingerprint,source,manifest) VALUES('maintain',?,?,?)`, program.Fingerprint(), testModule, string(metadata)); err != nil {
		t.Fatal(err)
	}
	identity, _ := json.Marshal(map[string]any{"version": 3, "driverKey": "maintain", "driverFingerprint": program.Fingerprint(), "owner": "private-owner", "query": "https://private.example/task"})
	if _, err = db.Exec(`INSERT INTO proxy_video_tasks(public_id,upstream_video_id,site_url,token_value,direct_identity,accounting_state) VALUES('video_direct_cli','','','',?,'{"version":1}')`, string(identity)); err != nil {
		t.Fatal(err)
	}
	lookup := func(name string) (string, bool) {
		if name != "TEST_MAINTENANCE_DSN" {
			t.Fatalf("read an unselected environment variable %s", name)
		}
		return dbPath, true
	}
	base := []string{"--dialect", "sqlite", "--dsn-env", "TEST_MAINTENANCE_DSN"}
	var out, diagnostics bytes.Buffer
	args := append(append([]string{}, base...), "--mode", "inspect", "--archive", "maintain:"+program.Fingerprint(), "--capture", "video_direct_cli")
	if err = run(t.Context(), args, &out, &diagnostics, lookup); err != nil {
		t.Fatal(err)
	}
	var plan channelplugins.MaintenancePlan
	if json.Unmarshal(out.Bytes(), &plan) != nil || len(plan.Captures) != 1 || len(plan.Archives) != 1 {
		t.Fatalf("invalid inspect output %s", out.String())
	}
	for _, private := range []string{dbPath, "private-owner", "private.example", "export const"} {
		if strings.Contains(out.String(), private) || strings.Contains(diagnostics.String(), private) {
			t.Fatal("CLI exposed private data")
		}
	}
	planPath := filepath.Join(t.TempDir(), "plan.json")
	if err = os.WriteFile(planPath, out.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	args = append(append([]string{}, base...), "--plan", planPath)
	if err = run(t.Context(), args, &out, &diagnostics, lookup); err != nil || !strings.Contains(out.String(), `"applied": false`) {
		t.Fatalf("default check: %s %v", out.String(), err)
	}
	var count int
	if err = db.Get(&count, `SELECT COUNT(*) FROM channel_plugin_versions`); err != nil || count != 1 {
		t.Fatal("check changed archive")
	}
	out.Reset()
	args = append(args, "--mode", "apply")
	if err = run(t.Context(), args, &out, &diagnostics, lookup); !errors.Is(err, channelplugins.ErrMaintenanceOffline) || out.Len() != 0 {
		t.Fatalf("apply accepted missing stop acknowledgement %v", err)
	}
	args = append(args, "--ack-all-instances-stopped")
	if err = run(t.Context(), args, &out, &diagnostics, lookup); err != nil || !strings.Contains(out.String(), `"applied": true`) {
		t.Fatalf("apply: %s %v", out.String(), err)
	}
	if err = db.Get(&count, `SELECT COUNT(*) FROM channel_plugin_versions`); err != nil || count != 0 {
		t.Fatal("archive not reclaimed")
	}
	if err = db.Get(&count, `SELECT COUNT(*) FROM proxy_video_tasks`); err != nil || count != 0 {
		t.Fatal("capture not reclaimed")
	}
	out.Reset()
	if err = run(t.Context(), args, &out, &diagnostics, lookup); err == nil || out.Len() != 0 {
		t.Fatal("unknown/stale apply claimed success")
	}
}

func TestCLIRejectsBadPlansAndDoesNotEchoInputs(t *testing.T) {
	for _, raw := range []string{
		`{}`, `null`, `{"archives":[],"unknown":true}`, `{"captures":[{"id":"video_direct_test","snapshotSha256":"bad","source":"private-source"}]}`,
		`{"archives":[]} {"captures":[]}`, `{"archives":[],"archives":[]}`, `{"archives":[{"key":"test","key":"other","fingerprint":"bad"}]}`,
		strings.Repeat(" ", (64<<10)+1), `{broken`,
	} {
		t.Run(raw[:min(30, len(raw))], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "plan.json")
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			var out, diagnostics bytes.Buffer
			lookup := func(string) (string, bool) {
				t.Fatal("invalid plan reached database environment lookup")
				return "", false
			}
			err := run(t.Context(), []string{"--dialect", "sqlite", "--dsn-env", "TEST_DSN", "--plan", path}, &out, &diagnostics, lookup)
			if !errors.Is(err, channelplugins.ErrMaintenancePlan) || out.Len() != 0 {
				t.Fatalf("invalid plan accepted %v", err)
			}
		})
	}
	for _, args := range [][]string{
		{"--dsn-env", "postgres://USER:PASS@HOST/DB?private-option=hidden"},
		{"--mode=postgres://USER:PASS@HOST/DB?private-option=hidden"},
		{"--unexpected=private-password"},
		{"--dialect", "sqlite", "--dsn-env", "TEST_DSN", "--mode", "apply", "--plan", "private-path"},
		{"--dialect", "sqlite", "--dsn-env", "TEST_DSN", "--mode", "inspect"},
	} {
		var out, diagnostics bytes.Buffer
		err := run(t.Context(), args, &out, &diagnostics, func(string) (string, bool) { t.Fatal("invalid arguments reached environment lookup"); return "", false })
		if err == nil || out.Len() != 0 || strings.Contains(diagnostics.String(), "private-") || strings.Contains(publicError(err), "private-") {
			t.Fatalf("bad argument leaked or succeeded: %v %s", err, diagnostics.String())
		}
	}
	if publicError(errors.New("private-password")) != "channel_plugin_maintenance_failed" {
		t.Fatal("raw error was exposed")
	}
}

func TestCLINeverCreatesMissingSQLiteDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "does-not-exist.db")
	var out, diagnostics bytes.Buffer
	err := run(context.Background(), []string{"--dialect", "sqlite", "--dsn-env", "TEST_DSN", "--mode", "inspect", "--archive", "maintain:" + strings.Repeat("0", 64)}, &out, &diagnostics, func(string) (string, bool) { return dbPath, true })
	if err == nil {
		t.Fatal("missing database succeeded")
	}
	if _, err = os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspection created a database %v", err)
	}
}
