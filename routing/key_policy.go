package routing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

const affinityTTL = 30 * time.Minute
const affinityCapacity = 4096

type affinityEntry struct {
	channelID int64
	directID  int64
	at        time.Time
}

type traceAffinity struct {
	mu      sync.Mutex
	entries map[string]affinityEntry
}

func newTraceAffinity() *traceAffinity {
	return &traceAffinity{entries: make(map[string]affinityEntry)}
}

func (a *traceAffinity) get(key string, now time.Time) affinityEntry {
	if key == "" {
		return affinityEntry{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.entries[key]
	if !ok || now.Sub(entry.at) >= affinityTTL {
		delete(a.entries, key)
		return affinityEntry{}
	}
	return entry
}

func (a *traceAffinity) remember(key string, entry affinityEntry, now time.Time) {
	if key == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, entry := range a.entries {
		if now.Sub(entry.at) >= affinityTTL {
			delete(a.entries, k)
		}
	}
	if len(a.entries) >= affinityCapacity {
		var oldestKey string
		var oldest time.Time
		for k, entry := range a.entries {
			if oldestKey == "" || entry.at.Before(oldest) {
				oldestKey, oldest = k, entry.at
			}
		}
		delete(a.entries, oldestKey)
	}
	entry.at = now
	a.entries[key] = entry
}

func effectiveStrategy(route store.TokenRoute, policy DownstreamRoutingPolicy) RouteRoutingStrategy {
	strategy := route.RoutingStrategy
	if policy.AccessPolicy != nil && !store.DefaultRoutingOverride(policy.AccessPolicy.RoutingStrategy) {
		strategy = policy.AccessPolicy.RoutingStrategy
	}
	return NormalizeRouteRoutingStrategy(strategy)
}

// The opaque client identity is already Key-namespaced at ingress. Include the
// route and both requested/mapped models so a mapping change cannot reuse a
// prior model's preference. No raw trace/Key is retained or exposed.
func affinityScope(route store.TokenRoute, model string, policy DownstreamRoutingPolicy) string {
	if policy.TraceIdentity == "" || store.EffectiveTraceStickyMode(route.TraceStickyMode, policy.AccessPolicy) != "prefer_previous_channel" {
		return ""
	}
	var mappings []store.DownstreamModelMapping
	if policy.AccessPolicy != nil {
		mappings = policy.AccessPolicy.ModelMappings
	}
	rawPolicy, _ := json.Marshal(mappings)
	routeMapping := ""
	if route.ModelMapping != nil {
		routeMapping = *route.ModelMapping
	}
	raw := policy.TraceIdentity + "\x00" + strconv.FormatInt(route.ID, 10) + "\x00" + model + "\x00" + routeMapping + "\x00" + string(rawPolicy)
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

// RecordSuccessfulSelection is called only for a completed upstream response,
// never on selection, a failed stream or a local admission rejection.
func (tr *TokenRouter) RecordSuccessfulSelection(selected *SelectedChannel) {
	if selected != nil && selected.AffinityScope != "" && tr.selector.affinity != nil {
		entry := affinityEntry{channelID: selected.Channel.ID}
		if selected.Direct != nil {
			entry.directID = selected.Direct.ChannelID
		}
		tr.selector.affinity.remember(selected.AffinityScope, entry, time.Now())
	}
}

// A previously successful channel that now fails loses its soft preference.
// Concurrent newer success on another channel is not removed.
func (tr *TokenRouter) RecordFailedSelection(selected *SelectedChannel) {
	if selected == nil || selected.AffinityScope == "" || tr.selector.affinity == nil {
		return
	}
	affinity := tr.selector.affinity
	affinity.mu.Lock()
	defer affinity.mu.Unlock()
	if entry, ok := affinity.entries[selected.AffinityScope]; ok && entry.channelID == selected.Channel.ID {
		delete(affinity.entries, selected.AffinityScope)
	}
}
