// metapi-go features/checkin — public barrel.
//
// Consumers should import only from here:
//   import { CheckinPage, useCheckinLogs, type CheckinLogRow } from '@/features/checkin'
//
// `export type` is used for all type-only re-exports (isolatedModules-safe).

// --- page + components ---
export { CheckinPage } from './components/checkin-page'
export { CheckinDetailSheet } from './components/checkin-detail-sheet'
export { ManualCheckinDialog } from './components/manual-checkin-dialog'
export { useCheckinColumns } from './components/checkin-columns'
export {
  FailureReasonBadge,
  FAILURE_CATEGORY_CONFIG,
  getFailureCategoryConfig,
  type FailureCategoryConfig,
} from './components/failure-reason-badge'

// --- checkin hooks + query keys ---
export {
  checkinQueryKeys,
  useCheckinLogs,
  useManualCheckin,
  useCheckinAccount,
  type UseCheckinLogsParams,
} from './api'

// --- checkin entity types + runtime schemas ---
export type {
  CheckinLogRow,
  CheckinLogInner,
  CheckinAccountInner,
  CheckinSiteInner,
  FailureReason,
  CheckinLogStatus,
  FailureReasonCategory,
  FailureReasonCode,
  CheckinSummary,
  TriggerCheckinAllResult,
  TriggerCheckinResult,
  CheckinRowActions,
} from './types'
export {
  checkinLogRowSchema,
  checkinLogInnerSchema,
  checkinAccountInnerSchema,
  checkinSiteInnerSchema,
  failureReasonSchema,
  checkinSummarySchema,
  triggerCheckinAllResultSchema,
  triggerCheckinResultSchema,
  CHECKIN_LOG_STATUS_VALUES,
  FAILURE_REASON_CATEGORIES,
  FAILURE_REASON_CODES,
} from './types'

// --- URL search schema + helpers ---
export {
  checkinSearchSchema,
  type CheckinSearch,
  DEFAULT_CHECKIN_PAGE_SIZE,
  getCheckinSearchDefaultValues,
  readCheckinSearchFromUrl,
  parseFilterValues,
  buildCheckinSearchString,
} from './lib/checkin-schema'

// --- time helpers ---
export {
  parseServerUtcDateTime,
  formatCheckinLogTime,
  formatDateTimeMinuteLocal,
  toLocalDatetimeInputValue,
  localDatetimeInputToEpochMs,
} from './lib/checkin-time'
