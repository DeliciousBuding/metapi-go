package admin

import (
	"net/http"

	"github.com/deliciousbuding/metapi-go/service/settingsmigration"
)

// GET /api/settings/migration/preview
func (h *settingsHandler) previewSettingsMigration(w http.ResponseWriter, r *http.Request) {
	plan, err := settingsmigration.BuildPlan(h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成设置迁移预览失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":               true,
		"currentVersion":        plan.CurrentVersion,
		"targetVersion":         plan.TargetVersion,
		"pending":               plan.Pending,
		"customCount":           plan.CustomCount,
		"legacyFieldsPreserved": plan.LegacyFieldsPreserved,
		"items":                 plan.Items,
	})
}

// POST /api/settings/migration/apply
func (h *settingsHandler) applySettingsMigration(w http.ResponseWriter, r *http.Request) {
	result, err := settingsmigration.Apply(h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "执行设置迁移失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":               true,
		"currentVersion":        result.CurrentVersion,
		"targetVersion":         result.TargetVersion,
		"pending":               result.Pending,
		"customCount":           result.CustomCount,
		"legacyFieldsPreserved": result.LegacyFieldsPreserved,
		"items":                 result.Items,
		"applied":               result.Applied,
	})
}
