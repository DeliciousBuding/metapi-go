import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Notice } from '@/components/ui/notice'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import type { DownstreamAccessPolicy } from '../types'

type Quota = NonNullable<DownstreamAccessPolicy['quota']>

export function AccessPolicyQuota({
  id,
  quota,
  onChange,
}: {
  id: string
  quota: DownstreamAccessPolicy['quota']
  onChange: (quota: DownstreamAccessPolicy['quota']) => void
}) {
  const { t } = useTranslation()
  const historyMissingBefore = quota?.historyMissingBefore
  const historyMissing = historyMissingBefore !== undefined
  function updateQuota(patch: Partial<Quota>) {
    onChange({ period: { type: 'all_time' }, ...quota, ...patch })
  }
  function selectPeriod(mode: string) {
    if (mode === 'past_duration') {
      updateQuota({
        period: { type: mode, pastDuration: { value: 1, unit: 'hour' } },
      })
    } else if (mode === 'day' || mode === 'month') {
      updateQuota({
        period: { type: 'calendar_duration', calendarDuration: { unit: mode } },
      })
    } else {
      updateQuota({ period: { type: 'all_time' } })
    }
  }
  const periodMode =
    quota?.period.type === 'calendar_duration'
      ? quota.period.calendarDuration?.unit
      : quota?.period.type
  return (
    <div className='space-y-3 border-t pt-4'>
      <div className='flex items-center justify-between gap-3'>
        <Label htmlFor={`${id}-quota`}>
          {t('settings.downstream.keys.access.quota')}
        </Label>
        <Switch
          id={`${id}-quota`}
          checked={quota !== undefined}
          disabled={historyMissing}
          onCheckedChange={(enabled) =>
            onChange(enabled ? { period: { type: 'all_time' } } : undefined)
          }
        />
      </div>
      {historyMissing && (
        <Notice tone='warning' role='status'>
          <p className='font-medium'>
            {t('settings.downstream.keys.access.historyTitle')}
          </p>
          <p>
            {t('settings.downstream.keys.access.historyHint', {
              date: new Date(historyMissingBefore).toLocaleString(),
            })}
          </p>
        </Notice>
      )}
      {quota && (
        <>
          <p className='text-muted-foreground text-xs'>
            {t('settings.downstream.keys.access.quotaHint')}
          </p>
          <div className='grid gap-3 sm:grid-cols-3'>
            {(['requests', 'totalTokens', 'cost'] as const).map((field) => (
              <div key={field} className='space-y-1.5'>
                <Label htmlFor={`${id}-${field}`}>
                  {t(`settings.downstream.keys.access.${field}`)}
                </Label>
                <Input
                  id={`${id}-${field}`}
                  type='number'
                  min={0}
                  step={field === 'cost' ? 'any' : 1}
                  value={quota[field] ?? ''}
                  placeholder={t('settings.common.unlimited')}
                  onChange={(event) =>
                    updateQuota({
                      [field]:
                        event.target.value === ''
                          ? undefined
                          : Number(event.target.value),
                    })
                  }
                />
              </div>
            ))}
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor={`${id}-period`}>
              {t('settings.downstream.keys.access.period')}
            </Label>
            <Select
              value={periodMode}
              onValueChange={(mode) => {
                if (mode) selectPeriod(mode)
              }}
            >
              <SelectTrigger id={`${id}-period`} className='w-full'>
                <SelectValue>
                  {t(
                    `settings.downstream.keys.access.periods.${periodMode ?? 'all_time'}`
                  )}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {(['all_time', 'past_duration', 'day', 'month'] as const).map(
                  (mode) => (
                    <SelectItem key={mode} value={mode}>
                      {t(`settings.downstream.keys.access.periods.${mode}`)}
                    </SelectItem>
                  )
                )}
              </SelectContent>
            </Select>
          </div>
          {quota.period.type === 'past_duration' && (
            <div className='grid grid-cols-2 gap-3'>
              <div className='space-y-1.5'>
                <Label htmlFor={`${id}-duration`}>
                  {t('settings.downstream.keys.access.duration')}
                </Label>
                <Input
                  id={`${id}-duration`}
                  type='number'
                  min={1}
                  step={1}
                  value={quota.period.pastDuration?.value ?? ''}
                  onChange={(event) =>
                    updateQuota({
                      period: {
                        type: 'past_duration',
                        pastDuration: {
                          value: Number(event.target.value),
                          unit: quota.period.pastDuration?.unit ?? 'hour',
                        },
                      },
                    })
                  }
                />
              </div>
              <div className='space-y-1.5'>
                <Label htmlFor={`${id}-unit`}>
                  {t('settings.downstream.keys.access.unit')}
                </Label>
                <Select
                  value={quota.period.pastDuration?.unit}
                  onValueChange={(unit) => {
                    if (
                      unit === 'minute' ||
                      unit === 'hour' ||
                      unit === 'day'
                    ) {
                      updateQuota({
                        period: {
                          type: 'past_duration',
                          pastDuration: {
                            value: quota.period.pastDuration?.value ?? 1,
                            unit,
                          },
                        },
                      })
                    }
                  }}
                >
                  <SelectTrigger id={`${id}-unit`} className='w-full'>
                    <SelectValue>
                      {t(
                        `settings.downstream.keys.access.units.${quota.period.pastDuration?.unit ?? 'hour'}`
                      )}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {(['minute', 'hour', 'day'] as const).map((unit) => (
                      <SelectItem key={unit} value={unit}>
                        {t(`settings.downstream.keys.access.units.${unit}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}
          <div className='space-y-1.5'>
            <Label htmlFor={`${id}-timezone`}>
              {t('settings.downstream.keys.access.timezone')}
            </Label>
            <Input
              id={`${id}-timezone`}
              value={quota.timezone ?? ''}
              placeholder='UTC'
              onChange={(event) =>
                updateQuota({ timezone: event.target.value || undefined })
              }
            />
            <p className='text-muted-foreground text-xs'>
              {t('settings.downstream.keys.access.timezoneHint')}
            </p>
          </div>
        </>
      )}
    </div>
  )
}
