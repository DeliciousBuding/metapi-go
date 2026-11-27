import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { SectionCard } from '@/components/common/section-card'
import { SectionError } from '@/components/common/section-error'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { checkinQueryKeys } from '@/features/checkin'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { SettingsFormActions } from '../../../components/settings-form-actions'
import { useSettingsForm } from '../../../hooks/use-settings-form'
import {
  collectChangedFields,
  hasChanges,
} from '../../../lib/collect-changed-fields'
import {
  useRuntimeSettings,
  useUpdateRuntimeSettings,
} from '../../../lib/runtime-settings'
import {
  DEFAULT_VALUES,
  deriveServerValues,
  schedulingSchema,
  schedulingToPayload,
  type SchedulingFormValues,
} from './scheduling-form-values'
import {
  BalanceRefreshScheduleFields,
  CheckinScheduleFields,
  LogCleanupScheduleFields,
  ModelSyncScheduleFields,
} from './scheduling-job-fields'
import { SchedulingMigrationCard } from './scheduling-migration-card'

const FORM_ID = 'settings-operations-scheduling-form'

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
      // new entries surface instead of waiting for the stale window to pass.
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
    const changed = collectChangedFields(values, baseline)
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
      <SectionCard
        title={t('settings.operations.scheduling.title')}
        description={t('settings.operations.scheduling.description')}
      >
        <p className='text-muted-foreground text-sm'>
          {t('settings.common.loading')}
        </p>
      </SectionCard>
    )
  }

  if (isError || !data) {
    return (
      <SectionError
        title={t('settings.operations.scheduling.title')}
        messageKey='settings.common.loadFailed'
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty

  return (
    <SectionCard
      title={t('settings.operations.scheduling.title')}
      description={t('settings.operations.scheduling.description')}
      hideHeaderCopy
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
      <SchedulingMigrationCard />
      {/* #1027: discoverability — map every upstream-touching monitoring job
          to its off switch so the "how do I turn health monitoring off"
          question is answered where users look for scheduling controls. */}
      <div className='border-primary/30 bg-primary/5 mb-4 space-y-2 rounded-lg border p-4'>
        <h3 className='text-sm font-medium'>
          {t('settings.operations.scheduling.healthMonitoring.title')}
        </h3>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('settings.operations.scheduling.healthMonitoring.description')}
        </p>
        <ul className='text-muted-foreground mt-2 list-inside list-disc space-y-1 text-xs'>
          <li>
            {t('settings.operations.scheduling.healthMonitoring.checkin')}
          </li>
          <li>
            {t('settings.operations.scheduling.healthMonitoring.balance')}
          </li>
          <li>{t('settings.operations.scheduling.healthMonitoring.probe')}</li>
          <li>
            {t('settings.operations.scheduling.healthMonitoring.passive')}
          </li>
        </ul>
      </div>
      <Form {...form}>
        <form
          id={FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <CheckinScheduleFields />
          <BalanceRefreshScheduleFields />
          <ModelSyncScheduleFields />
          <LogCleanupScheduleFields />

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
    </SectionCard>
  )
}
