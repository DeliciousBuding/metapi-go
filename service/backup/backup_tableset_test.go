package backup_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

// TestAllTablesCoversSchemaRegistry is the backup drift guard: every table the
// schema registry creates must either ship in a type=all backup or be excluded
// by name in store.BackupExcludedTables() with a recorded reason. A table that
// is in neither is a silent gap — its rows are dropped by every export and the
// restore looks complete while it is not.
func TestAllTablesCoversSchemaRegistry(t *testing.T) {
	inBackup := map[string]bool{}
	for _, table := range backupsvc.AllTables {
		if inBackup[table] {
			t.Fatalf("backup table set lists %s twice", table)
		}
		inBackup[table] = true
	}
	excluded := store.BackupExcludedTables()

	var silent []string
	for _, table := range store.SchemaTableNames() {
		if inBackup[table] {
			if reason, ok := excluded[table]; ok {
				t.Fatalf("%s is both exported and excluded (%q); pick one", table, reason)
			}
			continue
		}
		if _, ok := excluded[table]; !ok {
			silent = append(silent, table)
		}
	}
	sort.Strings(silent)
	if len(silent) > 0 {
		t.Fatalf("%d schema table(s) are silently dropped by the backup export: %v — "+
			"add each one to backupExcludedTables in store/tablesets.go with the reason it must "+
			"not be carried, or it ships in every type=all backup by default", len(silent), silent)
	}
}

// TestAllTablesIsDerivedFromTheRegistry pins the absence of a second copy: the
// exported set must be exactly store.BackupTableNames(), in the same FK-safe
// order, so a schema addition cannot be missed by forgetting to edit a list.
func TestAllTablesIsDerivedFromTheRegistry(t *testing.T) {
	if got, want := backupsvc.AllTables, store.BackupTableNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("backup.AllTables = %v, want store.BackupTableNames() = %v", got, want)
	}
}

// TestAccountsTablesScopeIsKnownAndSubset keeps the type=accounts scope honest:
// every member must be a registry table that a full backup also carries, and
// the order must be the registry's FK-safe order (one ordering owner).
func TestAccountsTablesScopeIsKnownAndSubset(t *testing.T) {
	registry := map[string]bool{}
	for _, table := range store.SchemaTableNames() {
		registry[table] = true
	}
	inBackup := map[string]bool{}
	for _, table := range backupsvc.AllTables {
		inBackup[table] = true
	}
	for _, table := range backupsvc.AccountsTables {
		if !registry[table] {
			t.Fatalf("accounts export scope lists %s, which is not in the schema registry", table)
		}
		if !inBackup[table] {
			t.Fatalf("accounts export scope lists %s, which the full backup set excludes", table)
		}
	}

	pos := map[string]int{}
	for i, table := range backupsvc.AllTables {
		pos[table] = i
	}
	for i := 1; i < len(backupsvc.AccountsTables); i++ {
		prev, cur := pos[backupsvc.AccountsTables[i-1]], pos[backupsvc.AccountsTables[i]]
		if prev > cur {
			t.Fatalf("accounts export order puts %s (pos %d) after %s (pos %d); "+
				"it must follow the registry FK-safe order",
				backupsvc.AccountsTables[i-1], prev, backupsvc.AccountsTables[i], cur)
		}
	}
}

// TestBuildPayloadMetadataListsExcludedTables is the operator-visibility gate:
// the export payload must state which registry tables it does not carry, with a
// non-empty reason, and must not claim a gap for a table it does carry.
func TestBuildPayloadMetadataListsExcludedTables(t *testing.T) {
	db := setupBackupServiceTestDB(t)

	for _, exportType := range []string{"all", "accounts", "preferences"} {
		payload, err := backupsvc.BuildPayload(db.DB, exportType)
		if err != nil {
			t.Fatalf("BuildPayload(%s): %v", exportType, err)
		}
		metadata, ok := payload["metadata"].(map[string]any)
		if !ok {
			t.Fatalf("metadata = %#v, want object", payload["metadata"])
		}
		if _, ok := metadata["exported_at"].(string); !ok {
			t.Fatalf("metadata.exported_at = %#v, want the pre-existing RFC3339 string", metadata["exported_at"])
		}
		if got := metadata["version"]; got != "1.0" {
			t.Fatalf("metadata.version = %#v, want the pre-existing \"1.0\"", got)
		}
		excluded, ok := metadata["excluded_tables"].(map[string]string)
		if !ok {
			t.Fatalf("metadata.excluded_tables = %#v, want map[table]reason", metadata["excluded_tables"])
		}

		tables, ok := payload["tables"].(map[string]any)
		if !ok {
			t.Fatalf("tables = %#v, want object", payload["tables"])
		}
		for carried := range tables {
			if reason, claimed := excluded[carried]; claimed {
				t.Fatalf("type=%s payload carries %s but metadata.excluded_tables claims it is missing (%q)",
					exportType, carried, reason)
			}
		}
		for table, reason := range excluded {
			if strings.TrimSpace(reason) == "" {
				t.Fatalf("type=%s metadata.excluded_tables[%s] has an empty reason", exportType, table)
			}
		}

		// The deliberate exclusions are reported for every export type.
		for table, wantReason := range store.BackupExcludedTables() {
			got, ok := excluded[table]
			if !ok {
				t.Fatalf("type=%s metadata.excluded_tables omits %s, which the backup never carries", exportType, table)
			}
			if got != wantReason {
				t.Fatalf("type=%s excluded reason for %s = %q, want the recorded %q", exportType, table, got, wantReason)
			}
		}
		if exportType == "all" {
			if len(excluded) != len(store.BackupExcludedTables()) {
				t.Fatalf("type=all metadata.excluded_tables = %d entries (%v), want exactly the %d deliberate exclusions",
					len(excluded), keysOf(excluded), len(store.BackupExcludedTables()))
			}
			continue
		}
		// A scoped export must also disclose the tables outside its scope.
		if _, ok := excluded["settings"]; exportType == "accounts" && !ok {
			t.Fatalf("type=accounts metadata.excluded_tables omits settings, which that scope does not carry")
		}
		if _, ok := excluded["accounts"]; exportType == "preferences" && !ok {
			t.Fatalf("type=preferences metadata.excluded_tables omits accounts, which that scope does not carry")
		}
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
