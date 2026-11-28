package store

import (
	"database/sql"
	"fmt"
	"io"
	"sort"
)

// readAllTables reads all migration tables from the source into memory.
// The table set is sourced from store.AllTableNames so it can never drift
// from the canonical schema.
func readAllTables(db *sql.DB) (map[string][]map[string]interface{}, error) {
	snapshot := make(map[string][]map[string]interface{})

	for _, table := range AllTableNames() {
		rows, err := db.Query(fmt.Sprintf(`SELECT * FROM "%s"`, table))
		if err != nil {
			// Table might not exist if migrations haven't created it
			snapshot[table] = nil
			continue
		}

		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("columns for %s: %w", table, err)
		}

		var tableRows []map[string]interface{}
		for rows.Next() {
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan %s: %w", table, err)
			}
			row := make(map[string]interface{})
			for i, col := range cols {
				row[col] = values[i]
			}
			tableRows = append(tableRows, row)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iter %s: %w", table, err)
		}

		snapshot[table] = tableRows
	}

	return snapshot, nil
}

// reportUncopiedSourceColumns logs every source column the copy will drop
// because the Go schema does not declare it. The target has nowhere to put
// such a value, so dropping is unavoidable — but it must never be silent.
func reportUncopiedSourceColumns(logw io.Writer, snapshot map[string][]map[string]interface{}) {
	for _, table := range AllTableNames() {
		spec, err := schemaColumns(table)
		if err != nil {
			continue
		}
		declared := make(map[string]bool, len(spec))
		for _, c := range spec {
			declared[c.name] = true
		}
		seen := make(map[string]bool)
		var dropped []string
		for _, row := range snapshot[table] {
			for col := range row {
				if declared[col] || seen[col] {
					continue
				}
				seen[col] = true
				dropped = append(dropped, col)
			}
		}
		if len(dropped) > 0 {
			sort.Strings(dropped)
			fmt.Fprintf(logw, "  Warning: %s: source column(s) %v are not in the target schema and are NOT copied\n", table, dropped)
		}
	}
}
