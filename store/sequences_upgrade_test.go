package store

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
)

const sequenceUpgradeTestTimestamp = "2026-09-09T00:00:00Z"

func seedExplicitSiteID(t *testing.T, db *DB, id int64) {
	t.Helper()
	if _, err := db.Exec(db.Rebind(`INSERT INTO sites (id, name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		id, "legacy-site-"+idString(id), "https://legacy-"+idString(id)+".example", "openai", "active",
		sequenceUpgradeTestTimestamp, sequenceUpgradeTestTimestamp); err != nil {
		t.Fatalf("seed explicit site id %d: %v", id, err)
	}
}

func setSiteSequence(t *testing.T, db *DB, value int64, called bool) {
	t.Helper()
	if _, err := db.Exec(`SELECT setval(pg_get_serial_sequence('sites', 'id'), $1, $2)`, value, called); err != nil {
		t.Fatalf("set sites sequence to %d (called=%t): %v", value, called, err)
	}
}

func insertSiteWithoutID(t *testing.T, db *DB) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(db.Rebind(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id`),
		"post-upgrade", "https://post-upgrade.example", "openai", "active",
		sequenceUpgradeTestTimestamp, sequenceUpgradeTestTimestamp).Scan(&id); err != nil {
		t.Fatalf("insert without id after AutoMigrate: %v", err)
	}
	return id
}

func idString(id int64) string {
	if id == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for id > 0 {
		i--
		buf[i] = byte('0' + id%10)
		id /= 10
	}
	return string(buf[i:])
}

// TestAutoMigrateAdvancesLegacyPostgresSequence is the upgrade-path regression:
// a database restored or imported with explicit ids can leave the serial
// sequence below MAX(id). AutoMigrate must repair that before the next ordinary
// INSERT; without the repair this test gets an id below the occupied range.
func TestAutoMigrateAdvancesLegacyPostgresSequence(t *testing.T) {
	db := openTestPG(t)
	pgtest.Reset(t, db.DB)

	seedExplicitSiteID(t, db, 10)
	setSiteSequence(t, db, 1, true)

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate upgrade: %v", err)
	}

	id := insertSiteWithoutID(t, db)
	if id <= 10 {
		t.Fatalf("insert after upgrade got id %d, want > occupied id 10", id)
	}
}

// TestAutoMigrateDoesNotRewindPostgresSequenceAheadOfRows pins the other side of
// the repair: a sequence may be ahead of MAX(id) because rows were deleted or
// ids were allocated. AutoMigrate must leave that watermark in place.
func TestAutoMigrateDoesNotRewindPostgresSequenceAheadOfRows(t *testing.T) {
	db := openTestPG(t)
	pgtest.Reset(t, db.DB)

	seedExplicitSiteID(t, db, 10)
	setSiteSequence(t, db, 100, true)

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate upgrade: %v", err)
	}

	var sequence int64
	if err := db.QueryRow(`SELECT pg_sequence_last_value(pg_get_serial_sequence('sites', 'id')::regclass)`).Scan(&sequence); err != nil {
		t.Fatalf("read sites sequence after AutoMigrate: %v", err)
	}
	if sequence != 100 {
		t.Fatalf("sequence last_value = %d after AutoMigrate, want the existing watermark 100", sequence)
	}

	id := insertSiteWithoutID(t, db)
	if id <= sequence {
		t.Fatalf("insert after upgrade got id %d, want > sequence watermark %d", id, sequence)
	}
}

// TestAutoMigrateMarksPostgresSequenceCalledAtMax covers the boundary where the
// sequence value equals MAX(id) but is_called=false. The next value would be
// the occupied id unless AutoMigrate marks the sequence as called.
func TestAutoMigrateMarksPostgresSequenceCalledAtMax(t *testing.T) {
	db := openTestPG(t)
	pgtest.Reset(t, db.DB)

	seedExplicitSiteID(t, db, 10)
	setSiteSequence(t, db, 10, false)

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate upgrade: %v", err)
	}

	id := insertSiteWithoutID(t, db)
	if id <= 10 {
		t.Fatalf("insert after upgrade got id %d, want > occupied id 10", id)
	}
}

// TestAutoMigratePreservesSQLiteSequenceBehavior verifies the PG repair is not
// applied to SQLite, whose AUTOINCREMENT bookkeeping already advances on
// explicit-id inserts.
func TestAutoMigratePreservesSQLiteSequenceBehavior(t *testing.T) {
	db := openTestSQLite(t)

	if _, err := db.Exec(`INSERT INTO sites (id, name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		10, "legacy-site-sqlite", "https://legacy-sqlite.example", "openai", "active",
		sequenceUpgradeTestTimestamp, sequenceUpgradeTestTimestamp); err != nil {
		t.Fatalf("seed explicit SQLite site id 10: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate SQLite upgrade: %v", err)
	}

	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"post-upgrade-sqlite", "https://post-upgrade-sqlite.example", "openai", "active",
		sequenceUpgradeTestTimestamp, sequenceUpgradeTestTimestamp)
	if err != nil {
		t.Fatalf("insert without id after SQLite AutoMigrate: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("read SQLite insert id: %v", err)
	}
	if id <= 10 {
		t.Fatalf("SQLite insert after upgrade got id %d, want > occupied id 10", id)
	}
}
