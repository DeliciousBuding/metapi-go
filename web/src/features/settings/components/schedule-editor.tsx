// metapi-go/features/settings/components — semantic schedule editor.
//
// Replaces the bare cron text inputs with preset controls: daily at HH:mm,
// every N hours (1–24), random-in-window (checkin only), or advanced raw
// cron. Unmappable/advanced crons keep their exact expression in `custom`
// mode. `window` has no deterministic cron and is only offered when
// `allowWindow` is set.

import { useTranslation } from 'react-i18next'

import type { ScheduleSpecV1 } from '@/lib/api'

import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import {
  cronToSchedule,
  scheduleToCron,
} from '../lib/schedule'

const INTERVAL_OPTIONS = Array.from({ length: 24 }, (_, index) => index + 1)

type ScheduleEditorProps = {
  value: ScheduleSpecV1 | undefined
  onChange: (spec: ScheduleSpecV1) => void
  /** Offer the random-in-window option (checkin only). */
  allowWindow?: boolean
  disabled?: boolean
}

export function ScheduleEditor({
  value,
  onChange,
  allowWindow,
  disabled,
}: ScheduleEditorProps) {
  const { t } = useTranslation()
  const kind = value?.kind ?? 'daily'
  const spec = value ?? { version: 1, kind: 'daily' as const, time: '08:00' }

  function switchKind(nextKind: ScheduleSpecV1['kind']) {
    if (nextKind === kind) {
      return
    }
    if (nextKind === 'custom') {
      onChange({
        version: 1,
        kind: 'custom',
        cron: scheduleToCron(value) ?? '',
      })
      return
    }
    if (nextKind === 'window') {
      onChange({
        version: 1,
        kind: 'window',
        windowStart: spec.kind === 'window' ? spec.windowStart : '00:00',
        windowEnd: spec.kind === 'window' ? spec.windowEnd : '23:59',
      })
      return
    }
    // Switching back to a semantic mode: reuse the mapped form of the current
    // cron when available, otherwise fall back to sensible defaults.
    const mapped =
      spec.kind === 'custom'
        ? cronToSchedule(spec.cron)
        : spec
    if (nextKind === 'interval') {
      onChange({
        version: 1,
        kind: 'interval',
        everyHours:
          mapped.kind === 'interval'
            ? mapped.everyHours
            : 6,
      })
      return
    }
    onChange({
      version: 1,
      kind: 'daily',
      time: mapped.kind === 'daily' ? mapped.time : '08:00',
    })
  }

  return (
    <div className='space-y-2'>
      <div className='flex flex-wrap items-center gap-2'>
        <Select
          value={kind}
          onValueChange={(next) => switchKind(next as ScheduleSpecV1['kind'])}
          disabled={disabled}
        >
          <SelectTrigger className='w-44'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='daily'>
              {t('settings.common.schedule.kinds.daily')}
            </SelectItem>
            <SelectItem value='interval'>
              {t('settings.common.schedule.kinds.interval')}
            </SelectItem>
            {allowWindow ? (
              <SelectItem value='window'>
                {t('settings.common.schedule.kinds.window')}
              </SelectItem>
            ) : null}
            <SelectItem value='custom'>
              {t('settings.common.schedule.kinds.custom')}
            </SelectItem>
          </SelectContent>
        </Select>
        {kind === 'daily' ? (
          <Input
            type='time'
            className='w-32 font-mono'
            value={spec.kind === 'daily' ? spec.time : '08:00'}
            disabled={disabled}
            onChange={(event) =>
              onChange({
                version: 1,
                kind: 'daily',
                time: event.target.value || '08:00',
              })
            }
          />
        ) : null}
        {kind === 'interval' ? (
          <Select
            value={String(spec.kind === 'interval' ? spec.everyHours : 6)}
            onValueChange={(next) =>
              onChange({
                version: 1,
                kind: 'interval',
                everyHours: Number(next),
              })
            }
            disabled={disabled}
          >
            <SelectTrigger className='w-32'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {INTERVAL_OPTIONS.map((hour) => (
                <SelectItem key={hour} value={String(hour)}>
                  {hour}h
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : null}
      </div>
      {kind === 'window' ? (
        <div className='flex flex-wrap items-center gap-2'>
          <Input
            type='time'
            className='w-32 font-mono'
            value={spec.kind === 'window' ? spec.windowStart : '00:00'}
            disabled={disabled}
            onChange={(event) =>
              onChange({
                version: 1,
                kind: 'window',
                windowStart: event.target.value || '00:00',
                windowEnd:
                  spec.kind === 'window' ? spec.windowEnd : '23:59',
              })
            }
          />
          <span className='text-xs text-muted-foreground'>→</span>
          <Input
            type='time'
            className='w-32 font-mono'
            value={spec.kind === 'window' ? spec.windowEnd : '23:59'}
            disabled={disabled}
            onChange={(event) =>
              onChange({
                version: 1,
                kind: 'window',
                windowStart:
                  spec.kind === 'window' ? spec.windowStart : '00:00',
                windowEnd: event.target.value || '23:59',
              })
            }
          />
        </div>
      ) : null}
      {kind === 'custom' ? (
        <Input
          className='w-full font-mono'
          value={spec.kind === 'custom' ? spec.cron : ''}
          disabled={disabled}
          placeholder='0 8 * * *'
          onChange={(event) =>
            onChange({
              version: 1,
              kind: 'custom',
              cron: event.target.value,
            })
          }
        />
      ) : null}
      <p className='text-xs text-muted-foreground'>
        {t(`settings.common.schedule.kinds.${kind}Hint`)}
      </p>
    </div>
  )
}
