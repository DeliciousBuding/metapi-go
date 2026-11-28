package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
)

// importTSV21Backup handles the TS backup v2.1 branch of the import endpoint.
func (h *backupHandler) importTSV21Backup(w http.ResponseWriter, raw []byte) {
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid import data: %v", err))
		return
	}
	result, err := backupsvc.ImportTSV21(h.db, raw)
	if err != nil {
		writeError(w, backupImportErrorStatus(err), err.Error())
		return
	}

	response := map[string]any{
		"success":  true,
		"message":  "import completed",
		"imported": result.Imported,
	}
	if result.SkippedSettings > 0 {
		response["skippedSettings"] = result.SkippedSettings
	}
	if len(result.NewDownstreamApiKeys) > 0 {
		response["newDownstreamApiKeys"] = result.NewDownstreamApiKeys
	}
	if len(result.Warnings) > 0 {
		response["warnings"] = result.Warnings
	}
	logImportedDownstreamKeys(h.db, result.NewDownstreamApiKeys)
	writeJSON(w, http.StatusOK, response)
}

// unwrapMagpieImportBody keeps the encrypted envelope intact while carrying
// its passphrase in the request body. The passphrase is never copied into a
// header, URL, response, or log.
func unwrapMagpieImportBody(raw []byte) ([]byte, string, bool, error) {
	if backupsvc.IsMagpieBackupPayload(raw) {
		return raw, "", true, nil
	}
	var wrapper struct {
		Passphrase *string         `json:"passphrase"`
		Data       json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &wrapper) != nil || len(wrapper.Data) == 0 || !backupsvc.IsMagpieBackupPayload(wrapper.Data) {
		return raw, "", false, nil
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return nil, "", false, fmt.Errorf("invalid import data: %w", err)
	}
	passphrase := ""
	if wrapper.Passphrase != nil {
		passphrase = *wrapper.Passphrase
	}
	if strings.TrimSpace(passphrase) == "" {
		return nil, "", true, errors.New("Magpie backup passphrase is required")
	}
	if len(passphrase) > 4096 {
		return nil, "", true, errors.New("Magpie backup passphrase is too long")
	}
	return wrapper.Data, passphrase, true, nil
}

func magpieOriginKey(r *http.Request) string {
	if key := strings.TrimSpace(r.Header.Get("X-External-Origin-Key")); key != "" {
		return key
	}
	return backupsvc.MagpieBackupOrigin
}

func magpieImportErrorMessage(err error) string {
	switch {
	case errors.Is(err, backupsvc.ErrMagpiePassphrase):
		return "Magpie backup could not be opened; check the passphrase"
	case errors.Is(err, backupsvc.ErrMagpieCorrupt):
		return "not a valid Magpie backup"
	case errors.Is(err, backupsvc.ErrMagpieVersion):
		return "unsupported Magpie backup version"
	default:
		return err.Error()
	}
}

// importMagpieBackup imports a passphrase-protected Magpie backup. The preview
// and transaction share the same compiled plan; replacement requires an
// explicit confirmation header, matching the other external importers.
func (h *backupHandler) importMagpieBackup(w http.ResponseWriter, r *http.Request, raw []byte, passphrase string) {
	originKey := magpieOriginKey(r)
	preview, err := backupsvc.PreviewMagpieBackup(h.db, raw, passphrase, originKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, magpieImportErrorMessage(err))
		return
	}
	if len(preview.Blocking) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "error": "Magpie backup has no provider a direct upstream grant can serve",
			"plan": preview, "blocking": preview.Blocking,
		})
		return
	}
	if len(preview.Removals) > 0 && r.Header.Get("X-Magpie-Replace-Origin") != "true" {
		writeJSON(w, http.StatusConflict, map[string]any{
			"success": false, "error": backupsvc.ErrMagpieReplacementRequired.Error(), "plan": preview,
		})
		return
	}
	counts, err := backupsvc.ImportMagpieBackup(h.db, raw, passphrase, originKey, r.Header.Get("X-Magpie-Replace-Origin") == "true", r.Header.Get("X-External-Replacement-Revision"))
	if err != nil {
		if errors.Is(err, backupsvc.ErrMagpieReplacementRequired) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, backupImportErrorStatus(err), magpieImportErrorMessage(err))
		return
	}
	routing.InvalidateCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "message": "Magpie backup import completed", "imported": counts,
		"notImported": preview.NotImported, "residuals": preview.Residuals,
		"skippedProviders": preview.SkippedProviders, "skippedGroups": preview.SkippedGroups,
	})
}

func (h *backupHandler) importOctopusV5Backup(w http.ResponseWriter, r *http.Request, raw []byte) {
	originKey := strings.TrimSpace(r.Header.Get("X-External-Origin-Key"))
	preview, err := backupsvc.PreviewOctopusV5(h.db, raw, originKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(preview.Blocking) > 0 {
		writeError(w, http.StatusBadRequest, "Octopus preview contains unsupported behavior that must be removed before import")
		return
	}
	if len(preview.Removals) > 0 && r.Header.Get("X-Octopus-Replace-Origin") != "true" {
		writeError(w, http.StatusConflict, "Source snapshot removes imported entries; review the preview and explicitly confirm origin replacement")
		return
	}
	importMode := strings.TrimSpace(r.Header.Get("X-Octopus-Import-Mode"))
	if importMode != "" && importMode != "channels-only" {
		writeError(w, http.StatusBadRequest, "unsupported Octopus import mode")
		return
	}
	if len(preview.NotImported) > 0 && importMode != "channels-only" {
		writeError(w, http.StatusBadRequest, "Octopus backup contains unsupported sections; explicitly select channels-only import after reviewing the preview")
		return
	}
	counts, err := backupsvc.ImportOctopusV5WithUnsupportedMode(h.db, raw, originKey, importMode == "channels-only", r.Header.Get("X-Octopus-Replace-Origin") == "true", r.Header.Get("X-External-Replacement-Revision"))
	if err != nil {
		if errors.Is(err, backupsvc.ErrOctopusReplacementRequired) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, backupImportErrorStatus(err), err.Error())
		return
	}
	routing.InvalidateCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"message":     "Octopus v5 import completed",
		"imported":    counts,
		"notImported": preview.NotImported,
	})
}

// importAxonHubV14Backup imports the channel graph an AxonHub backup describes.
// The preview and the transaction share one compiled plan, so an operator can
// only ever confirm what was shown: a payload whose channels no direct grant can
// serve is refused, and a re-import that would remove previously imported
// entries needs an explicit confirmation header.
func (h *backupHandler) importAxonHubV14Backup(w http.ResponseWriter, r *http.Request, raw []byte) {
	originKey := strings.TrimSpace(r.Header.Get("X-External-Origin-Key"))
	preview, err := backupsvc.PreviewAxonHubV14(h.db, raw, originKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(preview.Blocking) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success":  false,
			"error":    "AxonHub backup has no channel that a direct upstream grant can serve",
			"plan":     preview,
			"blocking": preview.Blocking,
		})
		return
	}
	if len(preview.Removals) > 0 && r.Header.Get("X-AxonHub-Replace-Origin") != "true" {
		writeJSON(w, http.StatusConflict, map[string]any{
			"success": false,
			"error":   backupsvc.ErrAxonHubReplacementRequired.Error(),
			"plan":    preview,
		})
		return
	}
	counts, err := backupsvc.ImportAxonHubV14(h.db, raw, originKey, r.Header.Get("X-AxonHub-Replace-Origin") == "true", r.Header.Get("X-External-Replacement-Revision"))
	if err != nil {
		if errors.Is(err, backupsvc.ErrAxonHubReplacementRequired) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, backupImportErrorStatus(err), err.Error())
		return
	}
	routing.InvalidateCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"message":     "AxonHub v1.4 import completed",
		"imported":    counts,
		"notImported": preview.NotImported,
		"residuals":   preview.Residuals,
		"skipped":     preview.SkippedChannels,
	})
}
