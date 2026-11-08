package channelplugins

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

var (
	ErrMaintenancePlan    = errors.New("channel_plugin_maintenance_plan_invalid")
	ErrMaintenanceOffline = errors.New("channel_plugin_maintenance_requires_all_instances_stopped")
	ErrMaintenanceCapture = errors.New("channel_plugin_maintenance_capture_changed_or_not_pending")
)

// A plan is an explicit selection, not a persistent revision axis or an age
// policy. It contains no source, credential, task identity or accounting data.
type MaintenanceVersion struct {
	Key         string `json:"key"`
	Fingerprint string `json:"fingerprint"`
}

type MaintenanceCapture struct {
	ID             string `json:"id"`
	SnapshotSHA256 string `json:"snapshotSha256"`
}

type MaintenancePlan struct {
	Archives []MaintenanceVersion `json:"archives"`
	Captures []MaintenanceCapture `json:"captures"`
}

type MaintenanceResult struct {
	Applied  bool `json:"applied"`
	Archives int  `json:"archives"`
	Captures int  `json:"captures"`
}

// Row locks protect against a late confirmation by the current host, but no
// database lock proves old hosts have stopped their network operations. Only
// the standalone offline CLI calls the apply path; there is no HTTP endpoint.
// The caller must stop ALL instances and preserve a backup before applying.
func MaintainArchivedVersions(ctx context.Context, db *sqlx.DB, plan MaintenancePlan, apply, allInstancesStopped bool) (MaintenanceResult, error) {
	var result MaintenanceResult
	if apply && !allInstancesStopped {
		return result, ErrMaintenanceOffline
	}
	if err := plan.Validate(); err != nil {
		return result, err
	}
	if db == nil {
		return result, ErrUnavailable
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = store.LockChannelPluginBindings(ctx, tx); err != nil {
		return result, err
	}
	// Readability is a prerequisite even if a selected capture would be removed.
	// Never erase damaged rows to manufacture a trustworthy zero-reference count.
	refs, err := store.ChannelPluginVersionReferences(ctx, tx)
	if err != nil {
		return result, err
	}
	var captures []maintenanceCaptureRow
	for _, selected := range plan.Captures {
		row, ref, err := loadMaintenanceCapture(ctx, tx, selected.ID, true)
		if err != nil {
			return result, err
		}
		if captureDigest(row) != selected.SnapshotSHA256 || refs[ref] < 1 {
			return result, ErrMaintenanceCapture
		}
		refs[ref]--
		captures = append(captures, row)
	}
	for _, selected := range plan.Archives {
		if refs[store.ChannelPluginVersionReference{Key: selected.Key, Fingerprint: selected.Fingerprint}] != 0 {
			return result, ErrVersionNotPrunable
		}
		version, err := store.LoadChannelPluginVersion(ctx, tx, selected.Key, selected.Fingerprint)
		if err != nil {
			return result, err
		}
		if _, err = validateArchivedProgram(ctx, version); err != nil {
			return result, err
		}
	}
	result.Archives, result.Captures = len(plan.Archives), len(plan.Captures)
	if !apply {
		// Rollback even the no-op row locks. A check never deletes or commits.
		return result, nil
	}
	for _, row := range captures {
		deleted, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM proxy_video_tasks WHERE id=? AND public_id=? AND upstream_video_id='' AND direct_identity=?`), row.RowID, row.ID, row.Identity)
		if err != nil {
			return MaintenanceResult{}, err
		}
		count, err := deleted.RowsAffected()
		if err != nil || count != 1 {
			return MaintenanceResult{}, ErrMaintenanceCapture
		}
	}
	for _, selected := range plan.Archives {
		deleted, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM channel_plugin_versions WHERE plugin_key=? AND fingerprint=?`), selected.Key, selected.Fingerprint)
		if err != nil {
			return MaintenanceResult{}, err
		}
		count, err := deleted.RowsAffected()
		if err != nil || count != 1 {
			return MaintenanceResult{}, ErrVersionNotPrunable
		}
	}
	if err = tx.Commit(); err != nil {
		return MaintenanceResult{}, err
	}
	result.Applied = true
	return result, nil
}

// Inspect only explicitly named rows. The returned digest is over the full
// non-credential row snapshot, not just its reference, so delete/recreate and
// changes to model, timestamps, ownership or accounting invalidate the plan.
func InspectMaintenancePlan(ctx context.Context, db *sqlx.DB, archives []MaintenanceVersion, captureIDs []string) (MaintenancePlan, error) {
	plan := MaintenancePlan{Archives: archives, Captures: []MaintenanceCapture{}}
	for _, id := range captureIDs {
		plan.Captures = append(plan.Captures, MaintenanceCapture{ID: id, SnapshotSHA256: strings.Repeat("0", 64)})
	}
	if err := plan.Validate(); err != nil {
		return MaintenancePlan{}, err
	}
	if db == nil {
		return MaintenancePlan{}, ErrUnavailable
	}
	tx, err := db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelSerializable})
	if err != nil {
		return MaintenancePlan{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = store.ChannelPluginVersionReferences(ctx, tx); err != nil {
		return MaintenancePlan{}, err
	}
	for _, selected := range archives {
		version, err := store.LoadChannelPluginVersion(ctx, tx, selected.Key, selected.Fingerprint)
		if err != nil {
			return MaintenancePlan{}, err
		}
		if _, err = validateArchivedProgram(ctx, version); err != nil {
			return MaintenancePlan{}, err
		}
	}
	for index, id := range captureIDs {
		row, _, err := loadMaintenanceCapture(ctx, tx, id, false)
		if err != nil {
			return MaintenancePlan{}, err
		}
		plan.Captures[index].SnapshotSHA256 = captureDigest(row)
	}
	return plan, tx.Commit()
}

func (plan MaintenancePlan) Validate() error {
	if len(plan.Archives)+len(plan.Captures) == 0 || len(plan.Archives) > 64 || len(plan.Captures) > 64 {
		return ErrMaintenancePlan
	}
	versions := map[MaintenanceVersion]bool{}
	for _, selected := range plan.Archives {
		if !contract.ValidKey(selected.Key) || !contract.ValidFingerprint(selected.Fingerprint) || versions[selected] {
			return ErrMaintenancePlan
		}
		versions[selected] = true
	}
	captures := map[string]bool{}
	for _, selected := range plan.Captures {
		if !strings.HasPrefix(selected.ID, "video_direct_") || len(selected.ID) <= len("video_direct_") || len(selected.ID) > 256 || !maintenanceCaptureID(selected.ID) || !contract.ValidFingerprint(selected.SnapshotSHA256) || captures[selected.ID] {
			return ErrMaintenancePlan
		}
		captures[selected.ID] = true
	}
	return nil
}

func maintenanceCaptureID(id string) bool {
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

type maintenanceCaptureRow struct {
	RowID          int64          `db:"id"`
	ID             string         `db:"public_id"`
	Identity       string         `db:"direct_identity"`
	Accounting     string         `db:"accounting_state"`
	RequestedModel sql.NullString `db:"requested_model"`
	ActualModel    sql.NullString `db:"actual_model"`
	ChannelID      sql.NullInt64  `db:"channel_id"`
	AccountID      sql.NullInt64  `db:"account_id"`
	Owner          sql.NullString `db:"owner"`
	Status         sql.NullString `db:"status_snapshot"`
	Response       sql.NullString `db:"upstream_response_meta"`
	LastStatus     sql.NullInt64  `db:"last_upstream_status"`
	LastPolled     sql.NullString `db:"last_polled_at"`
	Created        sql.NullString `db:"created_at"`
	Updated        sql.NullString `db:"updated_at"`
}

func captureDigest(row maintenanceCaptureRow) string {
	raw, _ := json.Marshal(row) // This struct only contains JSON-safe SQL scalars.
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func loadMaintenanceCapture(ctx context.Context, tx *sqlx.Tx, id string, lock bool) (maintenanceCaptureRow, store.ChannelPluginVersionReference, error) {
	var row maintenanceCaptureRow
	var ref store.ChannelPluginVersionReference
	query := `SELECT id,public_id,direct_identity,accounting_state,requested_model,actual_model,channel_id,account_id,owner,status_snapshot,upstream_response_meta,last_upstream_status,last_polled_at,created_at,updated_at FROM proxy_video_tasks WHERE public_id=? AND upstream_video_id='' AND site_url='' AND token_value=''`
	if lock {
		// Reuse the portable row lock already used by task accounting. It also
		// serializes with confirmation, which does not take the catalog lock.
		query = `UPDATE proxy_video_tasks SET public_id=public_id WHERE public_id=? AND upstream_video_id='' AND site_url='' AND token_value='' RETURNING id,public_id,direct_identity,accounting_state,requested_model,actual_model,channel_id,account_id,owner,status_snapshot,upstream_response_meta,last_upstream_status,last_polled_at,created_at,updated_at`
	}
	if err := tx.GetContext(ctx, &row, tx.Rebind(query), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return row, ref, ErrMaintenanceCapture
		}
		return row, ref, err
	}
	var identity struct {
		Version     int    `json:"version"`
		Key         string `json:"driverKey"`
		Fingerprint string `json:"driverFingerprint"`
	}
	if len(row.Identity) > 256<<10 || json.Unmarshal([]byte(row.Identity), &identity) != nil || identity.Version != 3 || !contract.ValidKey(identity.Key) || !contract.ValidFingerprint(identity.Fingerprint) {
		return row, ref, ErrMaintenanceCapture
	}
	var accounting store.VideoTaskAccounting
	decoder := json.NewDecoder(bytes.NewBufferString(row.Accounting))
	decoder.DisallowUnknownFields()
	var extra any
	if len(row.Accounting) > 4096 || decoder.Decode(&accounting) != nil || decoder.Decode(&extra) != io.EOF || accounting != (store.VideoTaskAccounting{Version: 1}) || row.Owner.Valid || row.AccountID.Valid || row.Status.Valid || row.Response.Valid || row.LastStatus.Valid || row.LastPolled.Valid {
		return row, ref, ErrMaintenanceCapture
	}
	return row, store.ChannelPluginVersionReference{Key: identity.Key, Fingerprint: identity.Fingerprint}, nil
}

func validateArchivedProgram(ctx context.Context, version store.ChannelPluginVersion) (*channelplugin.Program, error) {
	program, err := channelplugin.Compile(ctx, version.Source)
	if err != nil {
		return nil, err
	}
	manifest := program.Manifest()
	metadata, err := json.Marshal(manifest)
	if err != nil || !manifest.IsProvider() || manifest.Key != version.Key || program.Fingerprint() != version.Fingerprint || string(metadata) != version.Manifest {
		return nil, ErrBinding
	}
	return program, nil
}
