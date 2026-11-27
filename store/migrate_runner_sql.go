package store

import (
	"fmt"
	"slices"
	"strings"
)

// groupInsertStatements splits the flat row list into executable groups.
// Consecutive statements that share table + column list are merged into one
// multi-row INSERT of at most batchSize rows; batchSize <= 0 degenerates to
// single-row groups (the row-by-row TS default). buildStatements emits
// per-table runs in order, so grouping never reorders rows.
func groupInsertStatements(inserts []insertStmt, batchSize int) [][]insertStmt {
	if batchSize <= 0 {
		batchSize = 1
	}
	var groups [][]insertStmt
	var current []insertStmt
	flush := func() {
		if len(current) > 0 {
			groups = append(groups, current)
			current = nil
		}
	}
	for _, stmt := range inserts {
		if len(current) > 0 {
			sameShape := current[0].table == stmt.table && slices.Equal(current[0].columns, stmt.columns)
			if !sameShape || len(current) >= batchSize {
				flush()
			}
		}
		current = append(current, stmt)
	}
	flush()
	return groups
}

func quoteIdentPG(s string) string {
	return `"` + s + `"`
}

func quotedInsertColumns(columns []string) string {
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = quoteIdentPG(column)
	}
	return strings.Join(quoted, ", ")
}

// sqliteArg renders one bound value the way SQLite stores it. Booleans need
// the conversion: the driver binds a Go bool as the TEXT 'true'/'false', and
// an INTEGER-affinity column only converts numeric-looking text, so the value
// lands as text and stays text. A copy-migrated database then fails every
// `WHERE enabled = 1` comparison and hashes differently from the source it was
// copied from — while looking perfectly populated in a SELECT *.
func sqliteArg(value interface{}) interface{} {
	if b, ok := value.(bool); ok {
		if b {
			return int64(1)
		}
		return int64(0)
	}
	return value
}

func buildInsertSQLite(rows ...insertStmt) (string, []interface{}) {
	first := rows[0]
	tuple := "(" + strings.TrimSuffix(strings.Repeat("?, ", len(first.columns)), ", ") + ")"
	tuples := make([]string, len(rows))
	args := make([]interface{}, 0, len(rows)*len(first.columns))
	for i, row := range rows {
		tuples[i] = tuple
		for _, value := range row.values {
			args = append(args, sqliteArg(value))
		}
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", quoteIdentPG(first.table), quotedInsertColumns(first.columns), strings.Join(tuples, ", "))
	return query, args
}

func buildInsertPG(rows ...insertStmt) (string, []interface{}) {
	first := rows[0]
	// Single-row SQL quotes the table name; batch SQL uses the registry name directly.
	table := quoteIdentPG(first.table)
	args := first.values
	if len(rows) > 1 {
		table = first.table
		args = make([]interface{}, 0, len(rows)*len(first.columns))
		for _, row := range rows {
			args = append(args, row.values...)
		}
	}
	placeholders := make([]string, len(first.columns))
	tuples := make([]string, len(rows))
	for i := range rows {
		for j := range placeholders {
			placeholders[j] = fmt.Sprintf("$%d", i*len(placeholders)+j+1)
		}
		tuples[i] = "(" + strings.Join(placeholders, ", ") + ")"
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", table, quotedInsertColumns(first.columns), strings.Join(tuples, ", "))
	return query, args
}
