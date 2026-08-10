// Package observability is a true leaf (stdlib-only) holding cross-cutting
// counters that lower layers (scheduler) must record without importing
// handler/shared or app. Resolves package-boundaries §5.11: scheduler sat
// below app in the dependency chain (app → handler/proxy → scheduler), so
// it could not import either app or handler/shared to bump the DB-connection
// error metric. This leaf has no internal imports, breaking the cycle.
package observability

import "sync/atomic"

// dbConnErrorsTotal counts DB connection-budget / open errors
// (e.g. SQLSTATE 53300 too many connections for role). Canonical home for
// the counter; handler/shared delegates Record/Read/Reset here so the
// existing /metrics exposition and test reset continue to work unchanged.
var dbConnErrorsTotal atomic.Int64

// RecordDBConnError increments the DB connection-budget / open error counter.
func RecordDBConnError() { dbConnErrorsTotal.Add(1) }

// DBConnErrorsTotal returns the current counter value (Prometheus exposition / tests).
func DBConnErrorsTotal() int64 { return dbConnErrorsTotal.Load() }

// ResetDBConnErrorsForTest zeroes the counter for deterministic tests.
func ResetDBConnErrorsForTest() { dbConnErrorsTotal.Store(0) }
