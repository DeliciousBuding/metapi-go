// metapi-go features/checkin — entity types & runtime schemas.
//
// The checkin API (GET /api/checkin/logs) returns a flat JSON array whose
// elements are *nested* objects — the shape regression fix:
//
//   {
//     "checkin_logs": { id, accountId, status, message, reward, createdAt },
//     "accounts":      { id, username },
//     "sites":         { name, url },
//     "failureReason": { code, category, title, actionHint, detailHint } | null
//   }
//
// These Zod schemas parse each row defensively before feature code touches
// it, mirroring the accounts feature's `accountSchema.parse(row.original)`
// safety pattern. The category/code enums are kept in sync with the backend
// `service/checkin/failure_reason.go` `ClassifyFailureReason` classifier.

import { z } from 'zod'

const FAILURE_REASON_CATEGORIES = [
  'verification',
  'auth',
  'network',
  'site',
  'state',
  'unknown',
] as const

const checkinAccountInnerSchema = z.object({
  id: z.coerce.number().nullish().default(0),
  username: z.string().nullish().default(''),
})

const checkinLogInnerSchema = z.object({
  id: z.coerce.number(),
  accountId: z.coerce.number(),
  status: z.string().catch('failed'),
  message: z.string().nullish().default(null),
  reward: z.string().nullish().default(null),
  createdAt: z.string().nullish().default(''),
})

const checkinSiteInnerSchema = z.object({
  name: z.string().nullish().default(''),
  url: z.string().nullish().default(''),
})

const checkinSummarySchema = z.object({
  total: z.coerce.number().default(0),
  success: z.coerce.number().default(0),
  failed: z.coerce.number().default(0),
  skipped: z.coerce.number().default(0),
})

const failureReasonSchema = z
  .object({
    code: z.string().catch('unknown_error'),
    category: z.string().catch('unknown'),
    title: z.string().catch(''),
    actionHint: z.string().catch(''),
    detailHint: z.string().catch(''),
  })
  .nullish()
  .default(null)

// ---------------------------------------------------------------------------
// Enums — mirror service/checkin/failure_reason.go
// ---------------------------------------------------------------------------

export type FailureReasonCategory = (typeof FAILURE_REASON_CATEGORIES)[number]

// ---------------------------------------------------------------------------
// FailureReason — classified by ClassifyFailureReason on the write side and
// persisted as JSON in the `failure_reason` TEXT column (additive migration
// sc2_012). Parsed back to an object by the API layer; `null` for
// success/empty/garbage rows.
// ---------------------------------------------------------------------------

export type FailureReason = z.infer<typeof failureReasonSchema>

// ---------------------------------------------------------------------------
// Inner projections — the nested sub-objects inside each log row.
// ---------------------------------------------------------------------------

export const checkinLogRowSchema = z.object({
  checkin_logs: checkinLogInnerSchema,
  accounts: checkinAccountInnerSchema.nullish(),
  sites: checkinSiteInnerSchema.nullish(),
  failureReason: failureReasonSchema,
})
export type CheckinLogRow = z.infer<typeof checkinLogRowSchema>

// ---------------------------------------------------------------------------
// Trigger-all response — POST /api/checkin/trigger
// ---------------------------------------------------------------------------

export const triggerCheckinAllResultSchema = z.object({
  success: z.coerce.boolean().default(false),
  queued: z.coerce.boolean().default(false),
  status: z.string().catch('completed'),
  message: z.string().catch(''),
  summary: checkinSummarySchema.nullish(),
})

export const triggerCheckinResultSchema = z.object({
  success: z.coerce.boolean().default(false),
  status: z.string().catch('failed'),
  skipped: z.coerce.boolean().default(false),
  message: z.string().catch(''),
  reward: z.string().nullish().default(null),
  id: z.coerce.number().nullish(),
})

export interface CheckinRowActions {
  onViewDetail: (row: CheckinLogRow) => void
  onTriggerAccount: (row: CheckinLogRow) => void
}
