package store

// RowBool normalizes dialect-dependent boolean scans (SQLite 0/1, PG bool).
func RowBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int64:
		return b != 0
	case float64:
		return b != 0
	case string:
		return b == "true" || b == "1" || b == "t"
	}
	return false
}

// scannedBool accepts legacy SQLite representations as well as PostgreSQL booleans.
type scannedBool bool

func (b *scannedBool) Scan(value any) error {
	*b = scannedBool(RowBool(value))
	return nil
}
