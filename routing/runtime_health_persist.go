package routing

import (
	"encoding/json"
	"strconv"
	"time"
)

// ---- Persistence ----

// SiteRuntimeHealthPersistencePayload is the serialization format.
type SiteRuntimeHealthPersistencePayload struct {
	Version        int                                           `json:"version"`
	SavedAtMs      int64                                         `json:"savedAtMs"`
	GlobalBySiteID map[string]*SiteRuntimeHealthState            `json:"globalBySiteId"`
	ModelBySiteID  map[string]map[string]*SiteRuntimeHealthState `json:"modelBySiteId"`
}


// SettingsStore defines the interface for persisting runtime health state.
type SettingsStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

var healthSettingsStore SettingsStore

// SetHealthSettingsStore sets the settings store for health persistence.
func SetHealthSettingsStore(store SettingsStore) {
	healthStateMu.Lock()
	defer healthStateMu.Unlock()
	healthSettingsStore = store
}


// ---- Persistence ----

func shouldPersistSiteRuntimeHealthState(state *SiteRuntimeHealthState) bool {
	n := nowMs()
	lastTouchedAtMs := n
	for _, v := range []*int64{&state.LastUpdatedAtMs, state.LastFailureAtMs, state.LastSuccessAtMs, state.LastTransientFailureAtMs} {
		if v != nil && *v > lastTouchedAtMs {
			lastTouchedAtMs = *v
		}
	}
	if n-lastTouchedAtMs > SiteRuntimeHealthPersistStaleTTLMs {
		return false
	}
	if isRuntimeHealthBreakerOpen(state) {
		return true
	}
	if getDecayedSiteRuntimePenalty(state) >= SiteRuntimeHealthPersistMinPenalty {
		return true
	}
	if getRecentOutcomeSnapshot(state).SampleCount > 0.01 {
		return true
	}
	if state.LatencyEMAMs != nil && *state.LatencyEMAMs > 0 {
		return true
	}
	return n-lastTouchedAtMs <= SiteRuntimeHealthPersistIdleTTLMs
}

func cloneSiteRuntimeHealthState(state *SiteRuntimeHealthState) *SiteRuntimeHealthState {
	clone := &SiteRuntimeHealthState{
		PenaltyScore:            state.PenaltyScore,
		TransientFailureStreak:  state.TransientFailureStreak,
		RecentSuccessCount:      state.RecentSuccessCount,
		RecentFailureCount:      state.RecentFailureCount,
		RecentWindowUpdatedAtMs: state.RecentWindowUpdatedAtMs,
		BreakerLevel:            state.BreakerLevel,
		LastUpdatedAtMs:         state.LastUpdatedAtMs,
		LastProbeStatus:         state.LastProbeStatus,
		LastProbeModel:          state.LastProbeModel,
	}
	if state.LatencyEMAMs != nil {
		v := *state.LatencyEMAMs
		clone.LatencyEMAMs = &v
	}
	if state.FirstByteEMAMs != nil {
		v := *state.FirstByteEMAMs
		clone.FirstByteEMAMs = &v
	}
	if state.LastTransientFailureAtMs != nil {
		v := *state.LastTransientFailureAtMs
		clone.LastTransientFailureAtMs = &v
	}
	if state.BreakerUntilMs != nil {
		v := *state.BreakerUntilMs
		clone.BreakerUntilMs = &v
	}
	if state.LastFailureAtMs != nil {
		v := *state.LastFailureAtMs
		clone.LastFailureAtMs = &v
	}
	if state.LastSuccessAtMs != nil {
		v := *state.LastSuccessAtMs
		clone.LastSuccessAtMs = &v
	}
	if state.LastProbeAtMs != nil {
		v := *state.LastProbeAtMs
		clone.LastProbeAtMs = &v
	}
	if state.LastProbeLatencyMs != nil {
		v := *state.LastProbeLatencyMs
		clone.LastProbeLatencyMs = &v
	}
	if state.LastProbeError != nil {
		v := *state.LastProbeError
		clone.LastProbeError = &v
	}
	if state.LastProbeChannelID != nil {
		v := *state.LastProbeChannelID
		clone.LastProbeChannelID = &v
	}
	return clone
}

func scheduleSiteRuntimeHealthPersistence() {
	// Caller must hold healthStateMu (write lock). The AfterFunc clears the timer
	// under the same mutex so concurrent schedule/persist/flush paths are -race clean.
	if healthPersistTimer != nil {
		return
	}
	healthPersistTimer = time.AfterFunc(SiteRuntimeHealthPersistDebounceMs*time.Millisecond, func() {
		healthStateMu.Lock()
		healthPersistTimer = nil
		healthStateMu.Unlock()
		persistSiteRuntimeHealthState()
	})
}

func persistSiteRuntimeHealthState() {
	healthStateMu.Lock()
	if healthSettingsStore == nil {
		healthStateMu.Unlock()
		return
	}
	if healthPersistInFlight {
		healthStateMu.Unlock()
		return
	}
	store := healthSettingsStore
	healthPersistInFlight = true
	healthStateMu.Unlock()

	defer func() {
		healthStateMu.Lock()
		healthPersistInFlight = false
		healthStateMu.Unlock()
	}()

	// Build payload under read lock, then serialize outside the critical section.
	healthStateMu.RLock()
	payload := buildSiteRuntimeHealthPersistencePayload()
	healthStateMu.RUnlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = store.Set(SiteRuntimeHealthSettingKey, string(data))
}

func buildSiteRuntimeHealthPersistencePayload() SiteRuntimeHealthPersistencePayload {
	n := nowMs()
	payload := SiteRuntimeHealthPersistencePayload{
		Version:        1,
		SavedAtMs:      n,
		GlobalBySiteID: make(map[string]*SiteRuntimeHealthState),
		ModelBySiteID:  make(map[string]map[string]*SiteRuntimeHealthState),
	}

	for siteID, state := range siteRuntimeHealthStates {
		if shouldPersistSiteRuntimeHealthState(state) {
			payload.GlobalBySiteID[strconv.FormatInt(siteID, 10)] = cloneSiteRuntimeHealthState(state)
		}
	}
	for siteID, modelStates := range siteModelRuntimeHealthStates {
		persistedModels := make(map[string]*SiteRuntimeHealthState)
		for modelKey, state := range modelStates {
			if shouldPersistSiteRuntimeHealthState(state) {
				persistedModels[modelKey] = cloneSiteRuntimeHealthState(state)
			}
		}
		if len(persistedModels) > 0 {
			payload.ModelBySiteID[strconv.FormatInt(siteID, 10)] = persistedModels
		}
	}
	return payload
}

// ---- Reset / Flush / Load ----

// ResetSiteRuntimeHealthState clears all in-memory runtime health state.
func ResetSiteRuntimeHealthState() {
	healthStateMu.Lock()
	defer healthStateMu.Unlock()

	siteRuntimeHealthStates = make(map[int64]*SiteRuntimeHealthState)
	siteModelRuntimeHealthStates = make(map[int64]map[string]*SiteRuntimeHealthState)
	siteRuntimeHealthLoaded = false
	if healthPersistTimer != nil {
		healthPersistTimer.Stop()
		healthPersistTimer = nil
	}
	healthPersistInFlight = false
}

// ChannelRuntimeHealthRow describes a channel-row pair for health clearing.
type ChannelRuntimeHealthRow struct {
	SiteID            int64
	SourceModel       *string
	RouteModelPattern string
}

// ClearRuntimeHealthStatesForChannels clears the in-memory runtime health state
// for the site+model combinations of the given channel rows. Returns true if any
// state was cleared and persistence should be triggered.
func ClearRuntimeHealthStatesForChannels(rows []ChannelRuntimeHealthRow) bool {
	if len(rows) == 0 {
		return false
	}
	healthStateMu.Lock()
	defer healthStateMu.Unlock()

	cleared := false
	clearedSites := make(map[int64]bool)
	for _, row := range rows {
		if row.SiteID <= 0 {
			continue
		}
		if row.SourceModel != nil && *row.SourceModel != "" {
			modelKey := NormalizeChannelSourceModel(row.SourceModel)
			if modelKey != "" {
				if siteModels, ok := siteModelRuntimeHealthStates[row.SiteID]; ok {
					if _, exists := siteModels[modelKey]; exists {
						delete(siteModels, modelKey)
						cleared = true
					}
				}
			}
		}
		// Also clear global site health for sites we touched
		if !clearedSites[row.SiteID] {
			clearedSites[row.SiteID] = true
			if _, exists := siteRuntimeHealthStates[row.SiteID]; exists {
				delete(siteRuntimeHealthStates, row.SiteID)
				cleared = true
			}
		}
	}
	return cleared
}

// EnsureSiteRuntimeHealthStateLoaded lazy-loads health state from settings.
func EnsureSiteRuntimeHealthStateLoaded() error {
	if siteRuntimeHealthLoaded {
		return nil
	}
	healthStateMu.Lock()
	defer healthStateMu.Unlock()

	if siteRuntimeHealthLoaded {
		return nil
	}

	if healthSettingsStore != nil {
		raw, err := healthSettingsStore.Get(SiteRuntimeHealthSettingKey)
		if err == nil && raw != "" {
			var payload SiteRuntimeHealthPersistencePayload
			if err := json.Unmarshal([]byte(raw), &payload); err == nil && payload.Version == 1 {
				for key, state := range payload.GlobalBySiteID {
					siteID, parseErr := strconv.ParseInt(key, 10, 64)
					if parseErr == nil && siteID > 0 {
						siteRuntimeHealthStates[siteID] = state
					}
				}
				for siteIDKey, modelStates := range payload.ModelBySiteID {
					siteID, parseErr := strconv.ParseInt(siteIDKey, 10, 64)
					if parseErr == nil && siteID > 0 {
						hydrated := make(map[string]*SiteRuntimeHealthState)
						for modelKey, state := range modelStates {
							if modelKey != "" {
								hydrated[modelKey] = state
							}
						}
						if len(hydrated) > 0 {
							siteModelRuntimeHealthStates[siteID] = hydrated
						}
					}
				}
			}
		}
	}
	siteRuntimeHealthLoaded = true
	return nil
}

// FlushSiteRuntimeHealthPersistence flushes any pending persistence immediately.
func FlushSiteRuntimeHealthPersistence() {
	healthStateMu.Lock()
	if healthPersistTimer != nil {
		healthPersistTimer.Stop()
		healthPersistTimer = nil
	}
	healthStateMu.Unlock()
	persistSiteRuntimeHealthState()
}
