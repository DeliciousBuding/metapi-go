// metapi-go features/checkin — public barrel.
//
// Consumers should import only from here:
//   import { CheckinPage, useCheckinLogs, type CheckinLogRow } from '@/features/checkin'
//
// `export type` is used for all type-only re-exports (isolatedModules-safe).

// --- page + components ---

// --- checkin hooks + query keys ---
export { checkinQueryKeys } from './api'

// --- checkin entity types + runtime schemas ---
export { checkinLogRowSchema } from './types'

// --- URL search schema + helpers ---
export {
  checkinSearchSchema,
  readCheckinSearchFromUrl,
} from './lib/checkin-schema'

// --- time helpers ---
