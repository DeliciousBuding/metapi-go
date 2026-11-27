import { Link, type LinkProps } from '@tanstack/react-router'
import { ScrollText } from 'lucide-react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { buttonVariants } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

import { ScheduleEditor } from '../../../components/schedule-editor'
import type { SchedulingFormValues } from './scheduling-form-values'

function SchedulingToggle({
  name,
}: {
  name:
    | 'checkinEnabled'
    | 'balanceRefreshEnabled'
    | 'autoCreateModelRoutes'
    | 'logCleanupUsageLogsEnabled'
    | 'logCleanupProgramLogsEnabled'
}) {
  const { t } = useTranslation()
  const { control } = useFormContext<SchedulingFormValues>()
  return (
    <FormField
      control={control}
      name={name}
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
              {t(`settings.operations.scheduling.fields.${name}`)}
            </FormLabel>
            <FormDescription>
              {t(`settings.operations.scheduling.fields.${name}Hint`)}
            </FormDescription>
          </div>
        </FormItem>
      )}
    />
  )
}

function ScheduleField({
  name,
}: {
  name: 'checkinSchedule' | 'balanceRefreshSchedule' | 'logCleanupSchedule'
}) {
  const { t } = useTranslation()
  const { control } = useFormContext<SchedulingFormValues>()
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>
            {t(`settings.operations.scheduling.fields.${name}`)}
          </FormLabel>
          <FormControl>
            <ScheduleEditor
              value={field.value}
              onChange={field.onChange}
              allowWindow={name === 'checkinSchedule'}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export function CheckinScheduleFields() {
  const { t } = useTranslation()
  return (
    <div className='space-y-3 rounded-lg border p-4'>
      <h3 className='text-sm font-medium'>
        {t('settings.operations.scheduling.fields.checkinGroup')}
      </h3>
      {/* #1027: global check-in kill switch (per-account switches live
          on the Accounts page). */}
      <SchedulingToggle name='checkinEnabled' />
      <ScheduleField name='checkinSchedule' />
    </div>
  )
}

export function BalanceRefreshScheduleFields() {
  const { t } = useTranslation()
  return (
    <div className='space-y-3 rounded-lg border p-4'>
      <h3 className='text-sm font-medium'>
        {t('settings.operations.scheduling.fields.balanceGroup')}
      </h3>
      {/* #1027: global balance-refresh kill switch. */}
      <SchedulingToggle name='balanceRefreshEnabled' />
      <ScheduleField name='balanceRefreshSchedule' />
    </div>
  )
}

export function ModelSyncScheduleFields() {
  const { t } = useTranslation()
  const { control } = useFormContext<SchedulingFormValues>()
  return (
    <div className='space-y-3 rounded-lg border p-4'>
      <h3 className='text-sm font-medium'>
        {t('settings.operations.scheduling.fields.modelSyncGroup')}
      </h3>
      <SchedulingToggle name='autoCreateModelRoutes' />
      <FormField
        control={control}
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
              {t('settings.operations.scheduling.fields.modelSyncCronHint')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  )
}

export function LogCleanupScheduleFields() {
  const { t } = useTranslation()
  const { control } = useFormContext<SchedulingFormValues>()
  return (
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
      <ScheduleField name='logCleanupSchedule' />
      <FormField
        control={control}
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
      <SchedulingToggle name='logCleanupUsageLogsEnabled' />
      <SchedulingToggle name='logCleanupProgramLogsEnabled' />
    </div>
  )
}
