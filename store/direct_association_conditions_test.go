package store

import (
	"strings"
	"testing"
)

func TestAssociationConditionsMigrateLegacyItems(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ddl := strings.Replace(buildUpstreamGroupItemsDDL(DialectSQLite), "association_conditions TEXT NOT NULL DEFAULT '[]',", "", 1)
	if _, err = db.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	if err = AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err = AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.Get(&count, `SELECT COUNT(*) FROM pragma_table_info('upstream_group_items') WHERE name='association_conditions' AND dflt_value='''[]'''`); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("legacy item did not receive empty condition default")
	}
}

func TestAssociationConditionsRejectInvalidPersistence(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `[{"priority":1}]`, `[{"priority":1,"protocolOrder":[1],"condition":{"conditions":[{"field":"stream","operator":"eq","value":true,"future":true}]}}]`, `[{"priority":1,"protocolOrder":[999999]}]`, `[] trailing`} {
		var rules DirectAssociationConditions
		if rules.Scan(raw) == nil {
			t.Errorf("accepted invalid condition storage: %s", raw)
		}
	}
}
