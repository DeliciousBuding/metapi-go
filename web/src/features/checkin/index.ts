// metapi-go/features/checkin — public barrel.
//
// Cross-feature consumers import from here and not from a subdirectory: what
// is exported below is frozen against their call sites, and what is not
// exported is free to move.
// `CheckinPage` is deliberately not exported here: its route file loads it
// directly, so a feature that only needs the contract below does not pull
// the page into its own chunk.
//
// Type-only re-exports use `export type` (isolatedModules-safe).

// --- checkin hooks + query keys ---
export { checkinQueryKeys, fetchCheckinLogs, useManualCheckin } from './api'

// --- URL search schema + helpers ---
export {
  buildInitialCheckinLogsQuery,
  checkinSearchSchema,
  parseCheckinSearch,
} from './lib/checkin-schema'
