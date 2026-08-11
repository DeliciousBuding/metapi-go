// metapi-go/features/settings/sections/general/components — scheduling
// section. Cron + interval config for the three recurring jobs (checkin,
// balance refresh, log cleanup) and the cleanup retention/toggles. The
// legacy "test checkin" button is preserved (POST /api/checkin/trigger).

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { api } from '@/lib/api'

import { Button } from '@/components/ui/button'
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import {
  asBoolean,
  asNumber,
  asString,
  useRuntimeSettings,
  useUpdateRuntimeSettings,
} from '../../../lib/runtime-settings'

const FORM_ID = 'settings-general-scheduling-form'

const schedulingSchema = z.object({
  checkinScheduleMode: z.enum(['cron', 'interval']).optional(),
  checkinIntervalHours: z.coerce.number().int().min(1).max(24).optional(),
  checkinCron: z.string().optional(),
  balanceRefreshCron: z.string().optional(),
  logCleanupCron: z.string().optional(),
  logCleanupRetentionDays: z.coerce.number().int().min(1).optional(),
  logCleanupUsageLogsEnabled: z.boolean().optional(),
  logCleanupProgramLogsEnabled: z.boolean().optional(),
})

type SchedulingFormValues = z.infer<typeof schedulingSchema>

const INTERVAL_HOURS_OPTIONS = Array.from({ length: 24 }, (_, index) => index + 1)

export function SchedulingSection() {
  const { t } = useTranslation()
  const { data, isLoading } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const form = useForm<SchedulingFormValues>({
    resolver: zodResolver(schedulingSchema) as never,
    defaultValues: {
      checkinScheduleMode: 'cron',
      checkinIntervalHours: 6,
      checkinCron: '0 8 * * *',
      balanceRefreshCron: '0 * * * *',
      logCleanupCron: '0 6 * * *',
      logCleanupRetentionDays: 30,
      logCleanupUsageLogsEnabled: false,
      logCleanupProgramLogsEnabled: false,
    },
  })

  const checkinMode = form.watch('checkinScheduleMode')

  useEffect(() => {
    if (!data) {
      return
    }
    form.reset(
      {
        checkinScheduleMode:
          (asString(data.checkinScheduleMode) as 'cron' | 'interval') || 'cron',
        checkinIntervalHours: asNumber(data.checkinIntervalHours) ?? 6,
        checkinCron: asString(data.checkinCron),
        balanceRefreshCron: asString(data.balanceRefreshCron),
        logCleanupCron: asString(data.logCleanupCron),
        logCleanupRetentionDays: asNumber(data.logCleanupRetentionDays) ?? 30,
        logCleanupUsageLogsEnabled: asBoolean(data.logCleanupUsageLogsEnabled),
        logCleanupProgramLogsEnabled: asBoolean(data.logCleanupProgramLogsEnabled),
      },
      { keepDirtyValues: true },
    )
  }, [data, form])

  const triggerCheckinMutation = useMutation({
    mutationFn: async () => api.triggerCheckinAll(),
    onSuccess: () => toast.success(t('settings.general.scheduling.toast.checkinTriggered')),
    onError: () => toast.error(t('settings.general.scheduling.toast.checkinTriggerFailed')),
  })

  function onSubmit(values: SchedulingFormValues) {
    updateMutation.mutate(values as never, {
      onSuccess: () => toast.success(t('settings.general.scheduling.toast.saved')),
      onError: () => toast.error(t('settings.general.scheduling.toast.saveFailed')),
    })
  }

  if (isLoading) {
    return <SettingsSectionSkeleton />
  }

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
      <Form {...form}>
        <form
          id={FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='checkinScheduleMode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.general.scheduling.fields.checkinScheduleMode')}
                </FormLabel>
                <Select
                  value={field.value ?? 'cron'}
                  onValueChange={field.onChange}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue placeholder='cron' />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value='cron'>
                      {t('settings.general.scheduling.modeCron')}
                    </SelectItem>
                    <SelectItem value='interval'>
                      {t('settings.general.scheduling.modeInterval')}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <FormDescription>
                  {t('settings.general.scheduling.fields.checkinScheduleModeHint')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          {checkinMode === 'interval' ? (
            <FormField
              control={form.control}
              name='checkinIntervalHours'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.general.scheduling.fields.checkinIntervalHours')}
                  </FormLabel>
                  <Select
                    value={String(field.value ?? 6)}
                    onValueChange={(next) => field.onChange(Number(next))}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder='6' />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {INTERVAL_HOURS_OPTIONS.map((hour) => (
                        <SelectItem key={hour} value={String(hour)}>
                          {hour}h
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
          ) : null}
          {checkinMode !== 'interval' ? (
            <FormField
              control={form.control}
              name='checkinCron'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.general.scheduling.fields.checkinCron')}
                  </FormLabel>
                  <FormControl>
                    <Input {...field} value={field.value ?? ''} placeholder='0 8 * * *' />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          ) : null}
          <FormField
            control={form.control}
            name='balanceRefreshCron'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.general.scheduling.fields.balanceRefreshCron')}
                </FormLabel>
                <FormControl>
                  <Input {...field} value={field.value ?? ''} placeholder='0 * * * *' />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='logCleanupCron'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.general.scheduling.fields.logCleanupCron')}
                </FormLabel>
                <FormControl>
                  <Input {...field} value={field.value ?? ''} placeholder='0 6 * * *' />
                </FormControl>
                <FormDescription>
                  {t('settings.general.scheduling.fields.logCleanupCronHint')}
                </FormDescription>
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
                  {t('settings.general.scheduling.fields.logCleanupRetentionDays')}
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
                    {t('settings.general.scheduling.fields.logCleanupUsageLogsEnabled')}
                  </FormLabel>
                  <FormDescription>
                    {t('settings.general.scheduling.fields.logCleanupUsageLogsEnabledHint')}
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
                    {t('settings.general.scheduling.fields.logCleanupProgramLogsEnabled')}
                  </FormLabel>
                  <FormDescription>
                    {t('settings.general.scheduling.fields.logCleanupProgramLogsEnabledHint')}
                  </FormDescription>
                </div>
              </FormItem>
            )}
          />
          <Button
            type='submit'
            form={FORM_ID}
            disabled={updateMutation.isPending}
          >
            {updateMutation.isPending
              ? t('settings.common.saving')
              : t('settings.common.save')}
          </Button>
        </form>
      </Form>
    </SettingsSectionCard>
  )
}
