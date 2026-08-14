package routing

import "context"

// SnapshotDB defines the low-level DB operations for persisting route decision
// snapshots. The concrete implementation is service.ProxyRoutingStore
// (service/routing_store.go); the former high-level RouteDecisionSnapshotStore
// was removed as dead code — decision snapshots are written directly by the
// route-decision service through this interface.
type SnapshotDB interface {
	UpdateRouteDecisionSnapshot(ctx context.Context, routeID int64, snapshot string, refreshedAt string) error
	ClearRouteDecisionSnapshot(ctx context.Context, routeID int64) error
	ClearRouteDecisionSnapshots(ctx context.Context, routeIDs []int64) error
	ClearAllRouteDecisionSnapshots(ctx context.Context) error
	LoadRouteGroupSources(ctx context.Context, groupRouteIDs []int64) (map[int64][]int64, error)
}
