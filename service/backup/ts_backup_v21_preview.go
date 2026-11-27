package backup

import (
	"fmt"

	"github.com/deliciousbuding/metapi-go/store"
)

// TSV21TablePreview is the per-table preview plan for a v2.1 import.
type TSV21TablePreview struct {
	Rows        int64 `json:"rows"`
	ToInsert    int64 `json:"toInsert"`
	Duplicates  int64 `json:"duplicates"`
	SkippedRows int64 `json:"skippedRows"`
}

// TSV21PreviewResult is the full v2.1 import plan without writes.
type TSV21PreviewResult struct {
	Plan     map[string]TSV21TablePreview `json:"plan"`
	Warnings []string                     `json:"warnings,omitempty"`
}

// PreviewTSV21 computes the import plan for a v2.1 payload without writing
// anything. PK detection mirrors the tables-path preview: "id" for every table
// except "settings" which uses "key".
func PreviewTSV21(db *store.DB, raw []byte) (*TSV21PreviewResult, error) {
	parsed, err := ParseTSV21(raw)
	if err != nil {
		return nil, err
	}

	preview := &TSV21PreviewResult{
		Plan:     map[string]TSV21TablePreview{},
		Warnings: parsed.Warnings,
	}
	for _, table := range parsed.TableOrder {
		rows := parsed.Tables[table]
		plan := TSV21TablePreview{Rows: int64(len(rows))}
		if table == "settings" {
			plan.SkippedRows = parsed.SkippedSettings
		}
		pkColumn := "id"
		if table == "settings" {
			pkColumn = "key"
		}

		existing := map[string]bool{}
		var pkValues []string
		hasPKColumn := false
		for _, row := range rows {
			rawValue, present := row[pkColumn]
			if !present {
				plan.ToInsert++
				continue
			}
			hasPKColumn = true
			pk := fmt.Sprintf("%v", rawValue)
			if _, duplicate := existing[pk]; duplicate {
				plan.Duplicates++
				continue
			}
			existing[pk] = true
			pkValues = append(pkValues, pk)
		}
		if hasPKColumn && len(pkValues) > 0 {
			existingInDB := tsV21QueryExistingPKs(db, table, pkColumn, pkValues)
			for _, pk := range pkValues {
				if existingInDB[pk] {
					plan.Duplicates++
				} else {
					plan.ToInsert++
				}
			}
		}
		preview.Plan[table] = plan
	}
	return preview, nil
}
