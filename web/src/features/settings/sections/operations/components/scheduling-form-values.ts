import { z } from 'zod'

import type { RuntimeSettingsPayload, ScheduleSpecV1 } from '@/lib/api'

import type { RuntimeSettings } from '../../../lib/runtime-settings'
import {
  scheduleFromLegacy,
  scheduleToCron,
  specToLegacyMode,
} from '../../../lib/schedule'

const scheduleSpecSchema = z.discriminatedUnion('kind', [
  z.object({
    version: z.literal(1),
    kind: z.literal('daily'),
    time: z.string(),
  }),
  z.object({
    version: z.literal(1),
    kind: z.literal('interval'),
    everyHours: z.number().int().min(1).max(24),
  }),
  z.object({
    version: z.literal(1),
    kind: z.literal('window'),
    windowStart: z.string(),
    windowEnd: z.string(),
  }),
  z.object({
    version: z.literal(1),
    kind: z.literal('custom'),
    cron: z.string(),
  }),
])

export const schedulingSchema = z.object({
  checkinEnabled: z.boolean(),
  checkinSchedule: scheduleSpecSchema,
  balanceRefreshEnabled: z.boolean(),
  balanceRefreshSchedule: scheduleSpecSchema,
  modelSyncCron: z.string().min(1),
  autoCreateModelRoutes: z.boolean(),
  logCleanupSchedule: scheduleSpecSchema,
  logCleanupRetentionDays: z.coerce.number().int().min(1),
  logCleanupUsageLogsEnabled: z.boolean(),
  logCleanupProgramLogsEnabled: z.boolean(),
})

export type SchedulingFormValues = z.infer<typeof schedulingSchema>

export const DEFAULT_VALUES: SchedulingFormValues = {
  checkinEnabled: true,
  checkinSchedule: { version: 1, kind: 'daily', time: '08:00' },
  balanceRefreshEnabled: true,
  balanceRefreshSchedule: { version: 1, kind: 'interval', everyHours: 1 },
  modelSyncCron: '0 4 * * *',
  autoCreateModelRoutes: false,
  logCleanupSchedule: { version: 1, kind: 'daily', time: '06:00' },
  logCleanupRetentionDays: 30,
  logCleanupUsageLogsEnabled: false,
  logCleanupProgramLogsEnabled: false,
}

export function deriveServerValues(
  data: RuntimeSettings | undefined
): SchedulingFormValues | null {
  if (!data) {
    return null
  }
  const checkinSchedule =
    data.checkinSchedule ??
    scheduleFromLegacy({
      cron: data.checkinCron,
      mode: data.checkinScheduleMode,
      intervalHours: data.checkinIntervalHours,
      windowStart: data.checkinWindowStart,
      windowEnd: data.checkinWindowEnd,
    })
  const balanceSchedule =
    data.balanceRefreshSchedule ??
    scheduleFromLegacy({ cron: data.balanceRefreshCron })
  const logCleanupSchedule =
    data.logCleanupSchedule ?? scheduleFromLegacy({ cron: data.logCleanupCron })
  return {
    checkinEnabled: data.checkinEnabled ?? true,
    checkinSchedule,
    balanceRefreshEnabled: data.balanceRefreshEnabled ?? true,
    balanceRefreshSchedule: balanceSchedule,
    modelSyncCron: data.modelSyncCron ?? '0 4 * * *',
    autoCreateModelRoutes: data.autoCreateModelRoutes ?? false,
    logCleanupSchedule,
    logCleanupRetentionDays: data.logCleanupRetentionDays ?? 30,
    logCleanupUsageLogsEnabled: Boolean(data.logCleanupUsageLogsEnabled),
    logCleanupProgramLogsEnabled: Boolean(data.logCleanupProgramLogsEnabled),
  }
}

/** Project a changed schedule spec onto the legacy runtime payload keys. */
function projectSchedule(
  payload: RuntimeSettingsPayload,
  job: 'checkin' | 'balance' | 'log',
  spec: ScheduleSpecV1
) {
  if (job === 'checkin') {
    payload.checkinSchedule = spec
    const mode = specToLegacyMode(spec)
    payload.checkinScheduleMode = mode
    if (mode === 'interval' && spec.kind === 'interval') {
      payload.checkinIntervalHours = spec.everyHours
      payload.checkinCron = undefined
    } else if (mode === 'window' && spec.kind === 'window') {
      payload.checkinWindowStart = spec.windowStart
      payload.checkinWindowEnd = spec.windowEnd
      payload.checkinCron = undefined
    } else {
      payload.checkinCron = scheduleToCron(spec) ?? ''
    }
    return
  }
  if (job === 'balance') {
    payload.balanceRefreshSchedule = spec
    payload.balanceRefreshCron = scheduleToCron(spec) ?? ''
    return
  }
  payload.logCleanupSchedule = spec
  payload.logCleanupCron = scheduleToCron(spec) ?? ''
}

export function schedulingToPayload(
  changed: Partial<SchedulingFormValues>
): RuntimeSettingsPayload {
  const payload: RuntimeSettingsPayload = {}
  if (changed.checkinEnabled !== undefined) {
    payload.checkinEnabled = changed.checkinEnabled
  }
  if (changed.balanceRefreshEnabled !== undefined) {
    payload.balanceRefreshEnabled = changed.balanceRefreshEnabled
  }
  if (changed.checkinSchedule) {
    projectSchedule(payload, 'checkin', changed.checkinSchedule)
  }
  if (changed.balanceRefreshSchedule) {
    projectSchedule(payload, 'balance', changed.balanceRefreshSchedule)
  }
  if (changed.logCleanupSchedule) {
    projectSchedule(payload, 'log', changed.logCleanupSchedule)
  }
  if (changed.autoCreateModelRoutes !== undefined) {
    payload.autoCreateModelRoutes = changed.autoCreateModelRoutes
  }
  if (changed.modelSyncCron !== undefined) {
    payload.modelSyncCron = changed.modelSyncCron
  }
  if (changed.logCleanupRetentionDays !== undefined) {
    payload.logCleanupRetentionDays = changed.logCleanupRetentionDays
  }
  if (changed.logCleanupUsageLogsEnabled !== undefined) {
    payload.logCleanupUsageLogsEnabled = changed.logCleanupUsageLogsEnabled
  }
  if (changed.logCleanupProgramLogsEnabled !== undefined) {
    payload.logCleanupProgramLogsEnabled = changed.logCleanupProgramLogsEnabled
  }
  return payload
}
