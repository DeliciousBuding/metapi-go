package store

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

func verifyChecksums(srcDB *sql.DB, tgtDB *DB, snapshot map[string][]map[string]interface{}) error {
	for _, table := range AllTableNames() {
		srcRows := snapshot[table]

		// Runtime DB settings (db_type/db_url/db_ssl) are intentionally not
		// copied by buildSettings, so exclude them from the source side too —
		// otherwise settings always reports a false mismatch on real TS DBs.
		if table == "settings" {
			filtered := make([]map[string]interface{}, 0, len(srcRows))
			for _, row := range srcRows {
				if !runtimeDBSettingKeys[asString(row["key"])] {
					filtered = append(filtered, row)
				}
			}
			srcRows = filtered
		}

		// The column set actually present in the source is the data being
		// copied. A raw TypeScript database does not carry the Go additive
		// columns, so only hash this set on the target side too — otherwise
		// --verify always reports a false mismatch against a Go-created
		// target schema.
		sourceCols := make(map[string]struct{})
		for _, row := range srcRows {
			for col := range row {
				sourceCols[col] = struct{}{}
			}
		}

		// Restrict the hashed set to columns present on both sides: a source
		// column the Go schema never declared was not copied (and was reported
		// by reportUncopiedSourceColumns), so hashing it here would fail the
		// query instead of verifying the copy.
		if tgtCols, err := targetColumns(tgtDB, table); err == nil {
			present := make(map[string]struct{}, len(tgtCols))
			for _, c := range tgtCols {
				present[c] = struct{}{}
			}
			for col := range sourceCols {
				if _, ok := present[col]; !ok {
					delete(sourceCols, col)
				}
			}
		}

		srcCount := len(srcRows)

		var tgtCount int
		if err := tgtDB.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, table)).Scan(&tgtCount); err != nil {
			return fmt.Errorf("count %s: %w", table, err)
		}

		if srcCount != tgtCount {
			return fmt.Errorf("%s: row count mismatch (source=%d, target=%d)", table, srcCount, tgtCount)
		}

		// Compute hash of source and target for this table
		srcHash := hashRows(srcRows)
		tgtHash, err := hashPGTable(tgtDB, table, sourceCols)
		if err != nil {
			return fmt.Errorf("hash %s: %w", table, err)
		}

		if !bytes.Equal(srcHash, tgtHash) {
			return fmt.Errorf("%s: checksum mismatch (source=%x, target=%x)", table, srcHash, tgtHash)
		}
	}
	return nil
}

func hashRows(rows []map[string]interface{}) []byte {
	// The source snapshot comes from an unordered SELECT *, so sort rows by
	// their canonical serialization before hashing — otherwise the hash
	// depends on the source row order and never matches the target side
	// (which orders by primary key).
	sortedRows := append([]map[string]interface{}(nil), rows...)
	sort.SliceStable(sortedRows, func(i, j int) bool {
		return serializeRowForHash(sortedRows[i]) < serializeRowForHash(sortedRows[j])
	})

	h := sha256.New()
	for _, row := range sortedRows {
		keys := make([]string, 0, len(row))
		for k := range row {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h.Write([]byte(k))
			h.Write([]byte(fmt.Sprintf("%v", normalizeHashValue(row[k]))))
		}
	}
	return h.Sum(nil)
}

// serializeRowForHash renders a row as a deterministic "k=v,k=v,..." string so
// rows can be ordered independently of the source query plan.
func serializeRowForHash(row map[string]interface{}) string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(fmt.Sprintf("%v", normalizeHashValue(row[k])))
		b.WriteByte(',')
	}
	return b.String()
}

// hashPGTable hashes the target table restricted to the given allowed column
// set (the columns present in the source snapshot). A target created by
// store.AutoMigrate carries extra Go-only columns; hashing only the migrated
// column set keeps the checksum a pure data-copy verification that works for
// raw TypeScript sources as well as already-migrated ones.
func hashPGTable(db *DB, table string, allowedCols map[string]struct{}) ([]byte, error) {
	rows, err := db.Query(fmt.Sprintf(`SELECT * FROM "%s"`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Scan the target side into the same map form hashRows consumes, keeping
	// only the migrated columns, then reuse hashRows so both sides hash under
	// the identical canonical (sorted-key, sorted-row) serialization. An
	// ORDER BY in the query would be insufficient: the source snapshot's row
	// order is arbitrary, so the row order must come from the row content.
	colIndex := make(map[string]int, len(cols))
	for i, col := range cols {
		colIndex[col] = i
	}

	var scanned []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, len(cols))
		for col, i := range colIndex {
			if _, ok := allowedCols[col]; ok {
				row[col] = normalizeHashValue(values[i])
			}
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return hashRows(scanned), nil
}

// normalizeHashValue canonicalizes values that scan differently across
// dialects so checksums agree on both sides of a migration: SQLite stores
// booleans as INTEGER 0/1 while PostgreSQL scans them as bool true/false.
func normalizeHashValue(v interface{}) interface{} {
	if b, ok := v.(bool); ok {
		if b {
			return int64(1)
		}
		return int64(0)
	}
	return v
}
