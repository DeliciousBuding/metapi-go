package routing

import "context"

// SnapshotDB defines the DB operations needed to persist route decision
// snapshots. It is implemented by service.ProxyRoutingStore.
//
// The concrete snapshot store that previously lived alongside this interface
// (RouteDecisionSnapshotStore) was removed as dead code — its high-level
// orchestration is superseded by RouteDecisionService in decision.go, which
// drives these low-level operations through the DB contract.
type SnapshotDB interface {
	UpdateRouteDecisionSnapshot(ctx context.Context, routeID int64, snapshot string, refreshedAt string) error
	ClearRouteDecisionSnapshot(ctx context.Context, routeID int64) error
	ClearRouteDecisionSnapshots(ctx context.Context, routeIDs []int64) error
	ClearAllRouteDecisionSnapshots(ctx context.Context) error
	LoadRouteGroupSources(ctx context.Context, groupRouteIDs []int64) (map[int64][]int64, error)
}
