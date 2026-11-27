package store

import "github.com/jmoiron/sqlx"

// SelectMaps preserves driver values and column labels in heterogeneous projections.
func SelectMaps(db *sqlx.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.Queryx(db.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
