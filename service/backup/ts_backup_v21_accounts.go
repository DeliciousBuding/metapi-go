package backup

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

func parseTSV21AccountsSectionInto(raw json.RawMessage, parsed *TSV21Parsed) error {
	var section map[string]json.RawMessage
	if err := json.Unmarshal(raw, &section); err != nil || section == nil {
		return TSV21ClientError{Message: "invalid import data: accounts must be a JSON object"}
	}

	for key := range section {
		if !tsV21KnownAccountSections[key] {
			parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored unknown section accounts.%s", key))
		}
	}

	// Parent tables first, manualModels in the middle (model_availability
	// references accounts), children afterwards.
	for _, spec := range tsV21AccountTableSpecs[:5] {
		if err := appendTSV21SectionRows(section, spec, parsed); err != nil {
			return err
		}
	}
	if err := appendTSV21ManualModels(section, parsed); err != nil {
		return err
	}
	for _, spec := range tsV21AccountTableSpecs[5:] {
		if err := appendTSV21SectionRows(section, spec, parsed); err != nil {
			return err
		}
	}
	return nil
}

func appendTSV21SectionRows(section map[string]json.RawMessage, spec tsV21TableSpec, parsed *TSV21Parsed) error {
	rawRows, present := section[spec.section]
	if !present || isRawJSONNull(rawRows) {
		return nil
	}
	rows, err := parseTSV21SectionRows(spec, rawRows, &parsed.Warnings)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		parsed.Tables[spec.goTable] = rows
		parsed.TableOrder = append(parsed.TableOrder, spec.goTable)
	}
	return nil
}

func parseTSV21SectionRows(spec tsV21TableSpec, raw json.RawMessage, warnings *[]string) ([]map[string]any, error) {
	var rawRows []json.RawMessage
	if err := json.Unmarshal(raw, &rawRows); err != nil {
		return nil, TSV21ClientError{Message: fmt.Sprintf("invalid import data: accounts.%s must be an array", spec.section)}
	}
	if TSV21MaxRowsPerTable > 0 && len(rawRows) > TSV21MaxRowsPerTable {
		return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: accounts.%s exceeds the max rows of %d", spec.section, TSV21MaxRowsPerTable)}
	}

	unknownFieldCounts := map[string]int{}
	rows := make([]map[string]any, 0, len(rawRows))
	for i, rawRow := range rawRows {
		var row map[string]any
		if err := json.Unmarshal(rawRow, &row); err != nil || row == nil {
			*warnings = append(*warnings, fmt.Sprintf("ignored account row %d in %s: not a JSON object", i+1, spec.section))
			continue
		}

		converted, err := convertTSV21Row(spec, row, i+1)
		if err != nil {
			return nil, err
		}
		if TSV21MaxColumnsPerRow > 0 && len(converted) > TSV21MaxColumnsPerRow {
			return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: account row %d in %s exceeds the max columns of %d", i+1, spec.section, TSV21MaxColumnsPerRow)}
		}
		for field := range row {
			if !spec.fieldSet[field] {
				unknownFieldCounts[field]++
			}
		}
		if len(converted) > 0 {
			rows = append(rows, converted)
		}
	}

	for _, field := range sortedStringKeys(unknownFieldCounts) {
		count := unknownFieldCounts[field]
		if count > 1 {
			*warnings = append(*warnings, fmt.Sprintf("ignored unknown field %s.%s (%d rows)", spec.section, field, count))
		} else {
			*warnings = append(*warnings, fmt.Sprintf("ignored unknown field %s.%s", spec.section, field))
		}
	}
	return rows, nil
}

func convertTSV21Row(spec tsV21TableSpec, row map[string]any, rowIndex int) (map[string]any, error) {
	converted := make(map[string]any, len(spec.fields))
	for _, field := range spec.fields {
		value, present := row[field.tsName]
		if !present {
			continue
		}
		normalized, err := convertTSV21FieldValue(field, value)
		if err != nil {
			return nil, err
		}
		converted[field.goColumn] = normalized
	}
	for _, required := range spec.required {
		if _, ok := converted[required]; !ok {
			return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: account row %d in %s is missing required field %s", rowIndex, spec.section, required)}
		}
	}
	return converted, nil
}

func convertTSV21FieldValue(field tsV21Field, value any) (any, error) {
	switch field.kind {
	case tsV21Text:
		switch v := value.(type) {
		case nil:
			return nil, nil
		case string:
			if err := checkTSV21CellBytes(field.tsName, v); err != nil {
				return nil, err
			}
			return v, nil
		default:
			return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s must be a string or null", field.tsName)}
		}
	case tsV21JSONText:
		switch v := value.(type) {
		case nil:
			return nil, nil
		case string:
			if err := checkTSV21CellBytes(field.tsName, v); err != nil {
				return nil, err
			}
			return v, nil
		default:
			raw, err := json.Marshal(v)
			if err != nil {
				return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s cannot be serialized: %v", field.tsName, err)}
			}
			if err := checkTSV21CellBytes(field.tsName, string(raw)); err != nil {
				return nil, err
			}
			return string(raw), nil
		}
	case tsV21Int:
		switch v := value.(type) {
		case nil:
			if field.nullDefault != nil {
				return field.nullDefault, nil
			}
			return nil, nil
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v {
				return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s must be an integer", field.tsName)}
			}
			return int64(v), nil
		case int64:
			return v, nil
		default:
			return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s must be an integer", field.tsName)}
		}
	case tsV21Bool:
		switch v := value.(type) {
		case nil:
			if field.nullDefault != nil {
				return field.nullDefault, nil
			}
			return nil, nil
		case bool:
			return v, nil
		case float64:
			if v == 0 {
				return false, nil
			}
			if v == 1 {
				return true, nil
			}
		}
		return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s must be a boolean", field.tsName)}
	case tsV21Real:
		switch v := value.(type) {
		case nil:
			if field.nullDefault != nil {
				return field.nullDefault, nil
			}
			return nil, nil
		case float64:
			return v, nil
		case int64:
			return float64(v), nil
		}
		return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s must be a number", field.tsName)}
	default:
		return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s has an unrecognized type", field.tsName)}
	}
}

// appendTSV21ManualModels maps TS manualModels rows into model_availability
// rows with is_manual=true / available=true (mirroring the TS importer).
func appendTSV21ManualModels(section map[string]json.RawMessage, parsed *TSV21Parsed) error {
	rawRows, present := section["manualModels"]
	if !present || isRawJSONNull(rawRows) {
		return nil
	}
	var rawList []json.RawMessage
	if err := json.Unmarshal(rawRows, &rawList); err != nil {
		return TSV21ClientError{Message: "invalid import data: accounts.manualModels must be an array"}
	}
	if TSV21MaxRowsPerTable > 0 && len(rawList) > TSV21MaxRowsPerTable {
		return TSV21ClientError{Message: fmt.Sprintf("import failed: accounts.manualModels exceeds the max rows of %d", TSV21MaxRowsPerTable)}
	}

	checkedAt := time.Now().UTC().Format(time.RFC3339)
	rows := make([]map[string]any, 0, len(rawList))
	for i, rawRow := range rawList {
		var row map[string]any
		if err := json.Unmarshal(rawRow, &row); err != nil || row == nil {
			parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored accounts.manualModels row %d: not a JSON object", i+1))
			continue
		}
		accountID, err := convertTSV21FieldValue(tsV21Field{tsName: "accountId", goColumn: "account_id", kind: tsV21Int}, row["accountId"])
		if err != nil {
			return err
		}
		if accountID == nil {
			return TSV21ClientError{Message: fmt.Sprintf("import failed: accounts.manualModels row %d is missing required field accountId", i+1)}
		}
		modelName, ok := row["modelName"].(string)
		modelName = strings.TrimSpace(modelName)
		if !ok || modelName == "" {
			return TSV21ClientError{Message: fmt.Sprintf("import failed: accounts.manualModels row %d is missing required field modelName", i+1)}
		}
		for field := range row {
			if field != "accountId" && field != "modelName" {
				parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored unknown field manualModels.%s", field))
			}
		}
		rows = append(rows, map[string]any{
			"account_id": accountID,
			"model_name": modelName,
			"available":  true,
			"is_manual":  true,
			"latency_ms": nil,
			"checked_at": checkedAt,
		})
	}
	if len(rows) > 0 {
		parsed.Tables["model_availability"] = rows
		parsed.TableOrder = append(parsed.TableOrder, "model_availability")
	}
	return nil
}
