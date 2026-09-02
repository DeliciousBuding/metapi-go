package store

import (
	"strings"
	"testing"
)

// TestBackupExcludedTablesAreRegistryMembersWithReasons guards the exclusion
// side of the backup set: an entry naming a table the registry does not create
// is dead weight that hides a rename, and an entry without a reason is exactly
// the "forgot to add it" failure mode the derived set exists to prevent. The
// reason is also what operators see in metadata.excluded_tables, so it must say
// something.
func TestBackupExcludedTablesAreRegistryMembersWithReasons(t *testing.T) {
	registry := map[string]bool{}
	for _, table := range SchemaTableNames() {
		registry[table] = true
	}
	for table, reason := range backupExcludedTables {
		if !registry[table] {
			t.Fatalf("backupExcludedTables names %q, which the schema registry does not create; "+
				"remove the orphan or fix the name (an orphan exclusion silently un-guards the real table)", table)
		}
		if len(strings.TrimSpace(reason)) < 20 {
			t.Fatalf("backupExcludedTables[%q] reason = %q; it is shown to operators in "+
				"metadata.excluded_tables and must state why the table is not carried", table, reason)
		}
	}
	if len(backupExcludedTables) != len(BackupExcludedTables()) {
		t.Fatalf("BackupExcludedTables() returned %d entries, want %d (it must copy the whole set)",
			len(BackupExcludedTables()), len(backupExcludedTables))
	}
}

// TestBackupTableNamesAreFKSafe replays the import contract: a backup is
// imported in payload order inside one transaction, so every parent must
// precede the children that reference it or the restore dies on a foreign key.
func TestBackupTableNamesAreFKSafe(t *testing.T) {
	if err := schemaRegistryErr(); err != nil {
		t.Fatalf("schema registry: %v", err)
	}
	f := facts()
	names := BackupTableNames()
	pos := make(map[string]int, len(names))
	for i, name := range names {
		pos[name] = i
	}
	for _, name := range names {
		for _, parent := range f.parents[name] {
			p, ok := pos[parent]
			if !ok {
				continue // parent excluded from backups: no ordering constraint inside the set
			}
			if p > pos[name] {
				t.Fatalf("BackupTableNames() puts %s (pos %d) before its parent %s (pos %d); "+
					"an import replaying this order violates the foreign key", name, pos[name], parent, p)
			}
		}
	}
}

// TestBackupTableNamesPreserveRegistryOrder pins the derivation: the backup set
// must be AllTableNames() minus the exclusions, in the same order, so there is
// one topological order owner instead of a second hand-sorted list.
func TestBackupTableNamesPreserveRegistryOrder(t *testing.T) {
	excluded := map[string]bool{}
	for table := range backupExcludedTables {
		excluded[table] = true
	}
	var want []string
	for _, table := range AllTableNames() {
		if !excluded[table] {
			want = append(want, table)
		}
	}
	got := BackupTableNames()
	if len(got) != len(want) {
		t.Fatalf("BackupTableNames() returned %d tables, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("BackupTableNames()[%d] = %s, want %s (registry order)", i, got[i], want[i])
		}
	}
	if len(got)+len(backupExcludedTables) != len(SchemaTableNames()) {
		t.Fatalf("backup set (%d) + exclusions (%d) != registry (%d): every registry table must be "+
			"accounted for exactly once", len(got), len(backupExcludedTables), len(SchemaTableNames()))
	}
}
