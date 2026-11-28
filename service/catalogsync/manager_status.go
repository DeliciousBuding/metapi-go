package catalogsync

import (
	"context"
	"time"

	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
)

// SyncStatus is the status payload served to the panel.
type SyncStatus struct {
	Enabled     bool           `json:"enabled"`
	AutoSync    bool           `json:"autoSync"`
	IntervalMin int            `json:"intervalMin"`
	Snapshot    SnapshotStatus `json:"snapshot"`
	Sources     []Source       `json:"sources"`
}

// SnapshotStatus describes the published merged snapshot.
type SnapshotStatus struct {
	Source    string     `json:"source"`
	FetchedAt *time.Time `json:"fetchedAt"`
	Models    int        `json:"models"`
}

func (m *Manager) Status(ctx context.Context) (*SyncStatus, error) {
	m.mu.Lock()
	autoSync := m.autoEnabled
	m.mu.Unlock()

	intervalMin := 0
	if m.interval > 0 {
		intervalMin = int(m.interval / time.Minute)
	}

	sources, err := m.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	// Live provider statuses win over DB rows for the just-run attempt.
	live := m.provider.SourceStatuses()
	liveByID := map[int64]pricingcatalog.SourceReport{}
	liveByName := map[string]pricingcatalog.SourceReport{}
	for _, report := range live {
		liveByID[report.ID] = report
		liveByName[report.Name] = report
	}
	merged := make([]Source, 0, len(sources))
	for _, row := range sources {
		if liveReport, ok := liveByID[row.ID]; ok {
			row.LastSuccessAt = liveReport.LastSuccess
			row.LastError = strPtr(liveReport.LastError)
			row.LastCount = liveReport.ModelCount
			if !liveReport.AttemptedAt.IsZero() {
				row.LastAttemptAt = &liveReport.AttemptedAt
			}
		} else if liveReport, ok := liveByName[row.Name]; ok {
			row.LastSuccessAt = liveReport.LastSuccess
			row.LastError = strPtr(liveReport.LastError)
			row.LastCount = liveReport.ModelCount
			if !liveReport.AttemptedAt.IsZero() {
				row.LastAttemptAt = &liveReport.AttemptedAt
			}
		}
		merged = append(merged, row)
	}

	snapshot := m.provider.Snapshot()
	var fetchedAt *time.Time
	if snapshot != nil && !snapshot.FetchedAt.IsZero() {
		t := snapshot.FetchedAt
		fetchedAt = &t
	}
	snapSource := ""
	if snapshot != nil {
		snapSource = snapshot.Source
	}

	return &SyncStatus{
		Enabled:     true,
		AutoSync:    autoSync,
		IntervalMin: intervalMin,
		Snapshot: SnapshotStatus{
			Source:    snapSource,
			FetchedAt: fetchedAt,
			Models:    snapshotLen(snapshot),
		},
		Sources: merged,
	}, nil
}

func snapshotLen(snapshot *pricingcatalog.CatalogSnapshot) int {
	if snapshot == nil {
		return 0
	}
	return snapshot.Len()
}

// strPtr converts an empty string to nil so NULL columns round-trip.
func strPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
