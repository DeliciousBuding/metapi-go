// metapi-go/features/settings/sections/general/components — scheduling
// section. Semantic schedule controls (daily / interval / window / custom)
// for the three recurring jobs (checkin, balance refresh, log cleanup), the
// cleanup retention/toggles, and the one-click legacy-cron migration card.
// The legacy "test checkin" button is preserved (POST /api/checkin/trigger).

import { useMutation } from '@tanstack/react-query'
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
  logCleanupSchedule: scheduleSpecSchema,
  logCleanupRetentionDays: z.coerce.number().int().min(1),
  logCleanupUsageLogsEnabled: z.boolean(),
  logCleanupProgramLogsEnabled: z.boolean(),
})

type SchedulingFormValues = z.infer<typeof schedulingSchema>

const DEFAULT_VALUES: SchedulingFormValues = {
  checkinSchedule: { version: 1, kind: 'daily', time: '08:00' },
  balanceRefreshSchedule: { version: 1, kind: 'interval', everyHours: 1 },
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
  const { data, isLoading, isError, refetch } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const serverValues = deriveServerValues(data)
  const { form, baseline, syncFromServer } =
    useSettingsForm<SchedulingFormValues>({
      schema: schedulingSchema,
      defaultValues: DEFAULT_VALUES,
      serverValues,
    })

  const triggerCheckinMutation = useMutation({
    mutationFn: async () => api.triggerCheckinAll(),
    onSuccess: () =>
      toast.success(t('settings.general.scheduling.toast.checkinTriggered')),
    onError: () =>
      toast.error(t('settings.general.scheduling.toast.checkinTriggerFailed')),
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
        toast.success(t('settings.general.scheduling.toast.saved')),
      onError: () =>
        toast.error(t('settings.general.scheduling.toast.saveFailed')),
    })
  }

  if (isLoading) {
    return (
      <SettingsSectionCard
        title={t('settings.general.scheduling.title')}
        description={t('settings.general.scheduling.description')}
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
        title={t('settings.general.scheduling.title')}
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty

  return (
    <SettingsSectionCard
      title={t('settings.general.scheduling.title')}
      description={t('settings.general.scheduling.description')}
      actions={
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={triggerCheckinMutation.isPending}
          onClick={() => triggerCheckinMutation.mutate()}
        >
          {triggerCheckinMutation.isPending
            ? t('settings.general.scheduling.triggering')
            : t('settings.general.scheduling.triggerCheckin')}
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
            <h4 className='text-sm font-medium'>
              {t('settings.general.scheduling.fields.checkinGroup')}
            </h4>
            <FormField
              control={form.control}
              name='checkinSchedule'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.general.scheduling.fields.checkinSchedule')}
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
            <h4 className='text-sm font-medium'>
              {t('settings.general.scheduling.fields.balanceGroup')}
            </h4>
            <FormField
              control={form.control}
              name='balanceRefreshSchedule'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t(
                      'settings.general.scheduling.fields.balanceRefreshSchedule'
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
            <div className='flex items-center justify-between gap-2'>
              <h4 className='text-sm font-medium'>
                {t('settings.general.scheduling.fields.logCleanupGroup')}
              </h4>
              <Link
                to={
                  '/settings/system-info/program-logs' as
                    | LinkProps['to']
                    | (string & {})
                }
                className={cn(
                  buttonVariants({ variant: 'ghost', size: 'sm' }),
                  'text-muted-foreground h-7 px-2 text-xs'
                )}
              >
                <ScrollText className='size-3.5' />
                {t('settings.general.scheduling.viewProgramLogs')}
              </Link>
            </div>
            <FormField
              control={form.control}
              name='logCleanupSchedule'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.general.scheduling.fields.logCleanupSchedule')}
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
                      'settings.general.scheduling.fields.logCleanupRetentionDays'
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
                        'settings.general.scheduling.fields.logCleanupUsageLogsEnabled'
                      )}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'settings.general.scheduling.fields.logCleanupUsageLogsEnabledHint'
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
                        'settings.general.scheduling.fields.logCleanupProgramLogsEnabled'
                      )}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'settings.general.scheduling.fields.logCleanupProgramLogsEnabledHint'
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
        <h4 className='text-sm font-medium'>
          {t('settings.general.scheduling.migration.title')}
        </h4>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('settings.general.scheduling.migration.summary', {
            current: preview.currentVersion,
            target: preview.targetVersion,
            pending: preview.pending,
          })}
        </p>
        <ul className='text-muted-foreground mt-2 list-inside list-disc space-y-1 text-xs'>
          <li>
            {t('settings.general.scheduling.migration.customCount', {
              count: preview.customCount,
            })}
          </li>
          <li>{t('settings.general.scheduling.migration.legacyPreserved')}</li>
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
          : t('settings.general.scheduling.migration.apply')}
      </Button>
      <ConfirmDialog
        open={confirmOpen}
        title={t('settings.general.scheduling.migration.title')}
        description={t('settings.general.scheduling.migration.confirm')}
        confirmLabel={t('settings.general.scheduling.migration.apply')}
        cancelLabel={t('settings.common.cancel')}
        onConfirm={() => {
          setConfirmOpen(false)
          applyMutation.mutate(undefined, {
            onSuccess: () =>
              toast.success(t('settings.general.scheduling.migration.applied')),
            onError: () =>
              toast.error(
                t('settings.general.scheduling.migration.applyFailed')
              ),
          })
        }}
        onCancel={() => setConfirmOpen(false)}
      />
    </div>
  )
}
