package backup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

// AxonHubImportPreview is the safe, non-secret plan an operator reviews before
// committing. It never carries credentials, source URLs, channel names, or any
// other operator text.
type AxonHubImportPreview struct {
	Source          string                    `json:"source"`
	OriginKey       string                    `json:"originKey"`
	Version         string                    `json:"version"`
	Sections        map[string]int            `json:"sections"`
	Routable        map[string]int            `json:"routable"`
	NotImported     map[string]int            `json:"notImported,omitempty"`
	SkippedChannels []AxonHubSkippedChannel   `json:"skippedChannels,omitempty"`
	Residuals       []string                  `json:"residuals,omitempty"`
	Removals        map[string]int            `json:"removals,omitempty"`
	RemovalImpact   *upstream.DeletionPreview `json:"removalImpact,omitempty"`
	Blocking        []string                  `json:"blocking,omitempty"`
}

// IsAxonHubV14Payload reports whether the payload is an AxonHub v1.4 backup.
// Detection is by envelope only; ParseAxonHubSource does the real work.
func IsAxonHubV14Payload(raw []byte) bool {
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil || top == nil {
		return false
	}
	return bytes.Equal(bytes.TrimSpace(top["version"]), []byte(`"1.4"`)) && len(top["timestamp"]) > 0 &&
		len(top["channels"]) > 0 && len(top["models"]) > 0
}

// PreviewAxonHubV14 parses, compiles, and compares the result with what a
// previous import of the same origin created. It writes nothing.
func PreviewAxonHubV14(db *store.DB, raw []byte, originKey string) (*AxonHubImportPreview, error) {
	if err := validateOriginKey(originKey); err != nil {
		return nil, err
	}
	src, err := ParseAxonHubSource(raw)
	if err != nil {
		return nil, err
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil {
		return nil, err
	}
	preview := &AxonHubImportPreview{
		Source:      AxonHubV14Origin,
		OriginKey:   originKey,
		Version:     src.Version,
		Sections:    axonHubSourceSections(src),
		Routable:    axonHubPlanCounts(plan),
		NotImported: plan.notImported,
		Residuals:   plan.residuals,
		Blocking:    plan.blocking,
	}
	if len(plan.skipped) > 0 {
		preview.SkippedChannels = plan.skipped
	}
	if db != nil {
		tx, err := db.BeginTxx(context.Background(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		owned, err := mappedSourceIDs(tx, db, originKey)
		if err != nil {
			return nil, err
		}
		keep := axonHubPlanSourceIDs(plan)
		removalPlan, err := mappedSourceDeletionPlan(tx, originKey, keep, owned)
		if err != nil {
			return nil, err
		}
		preview.Removals = mappedSourceRemovalCounts(removalPlan, keep, owned)
		if len(preview.Removals) > 0 {
			preview.RemovalImpact = &removalPlan.Preview
		}
	}
	return preview, nil
}

func axonHubSourceSections(src *AxonHubSource) map[string]int {
	sections := map[string]int{
		"channels":      len(src.Channels),
		"models":        len(src.Models),
		"projects":      len(src.Projects),
		"apiKeys":       len(src.APIKeys),
		"modelPrices":   len(src.ChannelModelPrices),
		"systemConfigs": len(src.SystemConfigs),
		"usageRequests": src.UsageRequests,
		"usageLogs":     src.UsageLogs,
	}
	for section, count := range src.UnknownSections {
		sections["section:"+section] = count
	}
	return sections
}

func axonHubPlanCounts(plan *axonHubPlan) map[string]int {
	return map[string]int{
		"apiKeys":     len(plan.keys),
		"channels":    len(plan.channels),
		"credentials": len(plan.credentials),
		"models":      len(plan.models),
		"grants":      len(plan.grants),
		"routes":      len(plan.routes),
	}
}
