// metapi-go/features/settings/sections/basic/components — scheduling
// section. Semantic schedule controls (daily / interval / window / custom)
// for the three recurring jobs (checkin, balance refresh, log cleanup), the
// cleanup retention/toggles, and the one-click legacy-cron migration card.
// The legacy "test checkin" button is preserved (POST /api/checkin/trigger).

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, type LinkProps } from '@tanstack/react-router'
import { ScrollText } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button, buttonVariants } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { checkinQueryKeys } from '@/features/checkin'
import {
  api,
  type RuntimeSettingsPayload,
  type ScheduleSpecV1,
} from '@/lib/api'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { ScheduleEditor } from '../../../components/schedule-editor'
import { SettingsFormActions } from '../../../components/settings-form-actions'
import { SettingsSectionCard } from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'
import { useSettingsForm } from '../../../hooks/use-settings-form'
import {
  collectChangedFields,
  hasChanges,
} from '../../../lib/collect-changed-fields'
import {
  useApplySettingsMigration,
  useRuntimeSettings,
  useSettingsMigrationPreview,
  useUpdateRuntimeSettings,
  type RuntimeSettings,
} from '../../../lib/runtime-settings'
import {
  scheduleFromLegacy,
  scheduleToCron,
  specToLegacyMode,
} from '../../../lib/schedule'

const FORM_ID = 'settings-general-scheduling-form'

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

const schedulingSchema = z.object({
  checkinSchedule: scheduleSpecSchema,
  balanceRefreshSchedule: scheduleSpecSchema,
  modelSyncCron: z.string().min(1),
  logCleanupSchedule: scheduleSpecSchema,
  logCleanupRetentionDays: z.coerce.number().int().min(1),
  logCleanupUsageLogsEnabled: z.boolean(),
  logCleanupProgramLogsEnabled: z.boolean(),
})

type SchedulingFormValues = z.infer<typeof schedulingSchema>

const DEFAULT_VALUES: SchedulingFormValues = {
  checkinSchedule: { version: 1, kind: 'daily', time: '08:00' },
  balanceRefreshSchedule: { version: 1, kind: 'interval', everyHours: 1 },
  modelSyncCron: '0 4 * * *',
  logCleanupSchedule: { version: 1, kind: 'daily', time: '06:00' },
  logCleanupRetentionDays: 30,
  logCleanupUsageLogsEnabled: false,
  logCleanupProgramLogsEnabled: false,
}

function deriveServerValues(
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
    checkinSchedule,
    balanceRefreshSchedule: balanceSchedule,
    modelSyncCron: data.modelSyncCron ?? '0 4 * * *',
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

function schedulingToPayload(
  changed: Partial<SchedulingFormValues>
): RuntimeSettingsPayload {
  const payload: RuntimeSettingsPayload = {}
  if (changed.checkinSchedule) {
    projectSchedule(payload, 'checkin', changed.checkinSchedule)
  }
  if (changed.balanceRefreshSchedule) {
    projectSchedule(payload, 'balance', changed.balanceRefreshSchedule)
  }
  if (changed.logCleanupSchedule) {
    projectSchedule(payload, 'log', changed.logCleanupSchedule)
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

export function SchedulingSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data, isLoading, isError, refetch } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const serverValues = deriveServerValues(data)
  const { form, baseline, syncFromServer } =
    useSettingsForm<SchedulingFormValues>({
      schema: schedulingSchema,
      defaultValues: DEFAULT_VALUES,
      serverValues,
    })

  // POST /api/checkin/trigger answers 200 with `success:(failed==0)` plus a
  // per-account summary. A partial failure must be reported as one — with the
  // real counts — instead of the blanket "triggered" success the old code
  // toasted regardless of the envelope.
  const triggerCheckinMutation = useMutation({
    mutationFn: async () =>
      api.triggerCheckinAll() as Promise<{
        success?: boolean
        message?: string
        summary?: {
          total: number
          success: number
          failed: number
          skipped: number
        } | null
      }>,
    onSuccess: (result) => {
      // The triggered runs land in the /checkin log list — invalidate it so the
      // new entries surface instead of waiting for the stale window to pass
      // (W19-T1 N2).
      void queryClient.invalidateQueries({ queryKey: checkinQueryKeys.logs() })
      const summary = result?.summary
      if (summary && summary.failed > 0) {
        toast.warning(
          t('settings.operations.scheduling.toast.checkinPartialFailed'),
          {
            description: t('checkin.toast.summary', {
              total: summary.total,
              success: summary.success,
              failed: summary.failed,
              skipped: summary.skipped,
            }),
          }
        )
        return
      }
      if (result && result.success === false) {
        toast.error(
          result.message ||
            t('settings.operations.scheduling.toast.checkinTriggerFailed')
        )
        return
      }
      toast.success(t('settings.operations.scheduling.toast.checkinTriggered'))
    },
    onError: () =>
      toast.error(
        t('settings.operations.scheduling.toast.checkinTriggerFailed')
      ),
  })

  function onSubmit(values: SchedulingFormValues) {
    const changed = collectChangedFields(
      values as unknown as Record<string, unknown>,
      baseline as unknown as Record<string, unknown> | null
    ) as Partial<SchedulingFormValues>
    if (!hasChanges(changed)) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    updateMutation.mutate(schedulingToPayload(changed), {
      onSuccess: () =>
        toast.success(t('settings.operations.scheduling.toast.saved')),
      onError: () =>
        toast.error(t('settings.operations.scheduling.toast.saveFailed')),
    })
  }

  if (isLoading) {
    return (
      <SettingsSectionCard
        title={t('settings.operations.scheduling.title')}
        description={t('settings.operations.scheduling.description')}
      >
        <p className='text-muted-foreground text-sm'>
          {t('settings.common.loading')}
        </p>
      </SettingsSectionCard>
    )
  }

  if (isError || !data) {
    return (
      <SettingsSectionError
        title={t('settings.operations.scheduling.title')}
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty

  return (
    <SettingsSectionCard
      title={t('settings.operations.scheduling.title')}
      description={t('settings.operations.scheduling.description')}
      actions={
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={triggerCheckinMutation.isPending}
          onClick={() => triggerCheckinMutation.mutate()}
        >
          {triggerCheckinMutation.isPending
            ? t('settings.operations.scheduling.triggering')
            : t('settings.operations.scheduling.triggerCheckin')}
        </Button>
      }
    >
      <MigrationCard />
      <Form {...form}>
        <form
          id={FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <div className='space-y-3 rounded-lg border p-4'>
            <h3 className='text-sm font-medium'>
              {t('settings.operations.scheduling.fields.checkinGroup')}
            </h3>
            <FormField
              control={form.control}
              name='checkinSchedule'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.operations.scheduling.fields.checkinSchedule')}
                  </FormLabel>
                  <FormControl>
                    <ScheduleEditor
                      value={field.value}
                      onChange={field.onChange}
                      allowWindow
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='space-y-3 rounded-lg border p-4'>
            <h3 className='text-sm font-medium'>
              {t('settings.operations.scheduling.fields.balanceGroup')}
            </h3>
            <FormField
              control={form.control}
              name='balanceRefreshSchedule'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t(
                      'settings.operations.scheduling.fields.balanceRefreshSchedule'
                    )}
                  </FormLabel>
                  <FormControl>
                    <ScheduleEditor
                      value={field.value}
                      onChange={field.onChange}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='space-y-3 rounded-lg border p-4'>
            <h3 className='text-sm font-medium'>
              {t('settings.operations.scheduling.fields.modelSyncGroup')}
            </h3>
            <FormField
              control={form.control}
              name='modelSyncCron'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.operations.scheduling.fields.modelSyncCron')}
                  </FormLabel>
                  <FormControl>
                    <Input {...field} placeholder='0 4 * * *' />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'settings.operations.scheduling.fields.modelSyncCronHint'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='space-y-3 rounded-lg border p-4'>
            <div className='flex items-center justify-between gap-2'>
              <h3 className='text-sm font-medium'>
                {t('settings.operations.scheduling.fields.logCleanupGroup')}
              </h3>
              <Link
                to={
                  '/settings/operations/program-logs' as
                    | LinkProps['to']
                    | (string & {})
                }
                className={cn(
                  buttonVariants({ variant: 'ghost', size: 'sm' }),
                  'text-muted-foreground h-7 px-2 text-xs'
                )}
              >
                <ScrollText className='size-3.5' />
                {t('settings.operations.scheduling.viewProgramLogs')}
              </Link>
            </div>
            <FormField
              control={form.control}
              name='logCleanupSchedule'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t(
                      'settings.operations.scheduling.fields.logCleanupSchedule'
                    )}
                  </FormLabel>
                  <FormControl>
                    <ScheduleEditor
                      value={field.value}
                      onChange={field.onChange}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='logCleanupRetentionDays'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t(
                      'settings.operations.scheduling.fields.logCleanupRetentionDays'
                    )}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      type='number'
                      min={1}
                      placeholder='30'
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='logCleanupUsageLogsEnabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center gap-3'>
                  <FormControl>
                    <Checkbox
                      checked={Boolean(field.value)}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <div className='space-y-1'>
                    <FormLabel className='cursor-pointer'>
                      {t(
                        'settings.operations.scheduling.fields.logCleanupUsageLogsEnabled'
                      )}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'settings.operations.scheduling.fields.logCleanupUsageLogsEnabledHint'
                      )}
                    </FormDescription>
                  </div>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='logCleanupProgramLogsEnabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center gap-3'>
                  <FormControl>
                    <Checkbox
                      checked={Boolean(field.value)}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <div className='space-y-1'>
                    <FormLabel className='cursor-pointer'>
                      {t(
                        'settings.operations.scheduling.fields.logCleanupProgramLogsEnabled'
                      )}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'settings.operations.scheduling.fields.logCleanupProgramLogsEnabledHint'
                      )}
                    </FormDescription>
                  </div>
                </FormItem>
              )}
            />
          </div>

          <SettingsFormActions
            formId={FORM_ID}
            isDirty={isDirty}
            isPending={updateMutation.isPending}
            onReset={() =>
              syncFromServer(deriveServerValues(data) ?? DEFAULT_VALUES)
            }
          />
        </form>
      </Form>
      <FormNavigationGuard enabled={isDirty} />
    </SettingsSectionCard>
  )
}

function MigrationCard() {
  const { t } = useTranslation()
  const previewQuery = useSettingsMigrationPreview()
  const applyMutation = useApplySettingsMigration()
  const [confirmOpen, setConfirmOpen] = useState(false)

  const preview = previewQuery.data
  if (previewQuery.isError) {
    // Old backend without the migration endpoint — degrade silently.
    return null
  }
  if (!preview || preview.pending === 0) {
    return null
  }

  return (
    <div className='border-primary/30 bg-primary/5 mb-6 space-y-3 rounded-lg border p-4'>
      <div>
        <h3 className='text-sm font-medium'>
          {t('settings.operations.scheduling.migration.title')}
        </h3>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('settings.operations.scheduling.migration.summary', {
            current: preview.currentVersion,
            target: preview.targetVersion,
            pending: preview.pending,
          })}
        </p>
        <ul className='text-muted-foreground mt-2 list-inside list-disc space-y-1 text-xs'>
          <li>
            {t('settings.operations.scheduling.migration.customCount', {
              count: preview.customCount,
            })}
          </li>
          <li>
            {t('settings.operations.scheduling.migration.legacyPreserved')}
          </li>
        </ul>
      </div>
      <Button
        type='button'
        size='sm'
        disabled={applyMutation.isPending}
        onClick={() => setConfirmOpen(true)}
      >
        {applyMutation.isPending
          ? t('settings.common.saving')
          : t('settings.operations.scheduling.migration.apply')}
      </Button>
      <ConfirmDialog
        open={confirmOpen}
        title={t('settings.operations.scheduling.migration.title')}
        description={t('settings.operations.scheduling.migration.confirm')}
        confirmLabel={t('settings.operations.scheduling.migration.apply')}
        cancelLabel={t('settings.common.cancel')}
        onConfirm={() => {
          setConfirmOpen(false)
          applyMutation.mutate(undefined, {
            onSuccess: () =>
              toast.success(
                t('settings.operations.scheduling.migration.applied')
              ),
            onError: () =>
              toast.error(
                t('settings.operations.scheduling.migration.applyFailed')
              ),
          })
        }}
        onCancel={() => setConfirmOpen(false)}
      />
    </div>
  )
}
