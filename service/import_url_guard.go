package service

// importURLColumns lists, per table, the columns whose values become
// server-initiated outbound request targets. Rows admitted through bulk site
// import or backup import must satisfy the same IsForbiddenSiteTargetURL
// rules that the single-row create/update endpoints enforce (sites.go,
// site_endpoint_service.go); otherwise a crafted import payload can plant
// cloud-metadata / link-local targets that the data plane later dials
// (first-hop SSRF), bypassing the config-layer validation.
var importURLColumns = map[string][]string{
	"sites":              {"url", "external_checkin_url", "proxy_url"},
	"site_api_endpoints": {"url"},
}

// SanitizeImportedSiteRows drops imported rows whose outbound-target URL
// columns point at forbidden targets (cloud metadata / link-local). Empty or
// absent values pass (nullable columns; IsForbiddenSiteTargetURL is
// empty-tolerant). Returns the surviving rows and how many were dropped.
// Tables without guarded columns are returned unchanged.
func SanitizeImportedSiteRows(table string, rows []map[string]any) ([]map[string]any, int) {
	cols := importURLColumns[table]
	if len(cols) == 0 {
		return rows, 0
	}
	kept := make([]map[string]any, 0, len(rows))
	dropped := 0
	for _, row := range rows {
		forbidden := false
		for _, col := range cols {
			if v, ok := row[col].(string); ok && IsForbiddenSiteTargetURL(v) {
				forbidden = true
				break
			}
		}
		if forbidden {
			dropped++
			continue
		}
		kept = append(kept, row)
	}
	return kept, dropped
}
