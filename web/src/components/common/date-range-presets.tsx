// metapi-go/components/common — quick-pick presets for the datetime-local
// range filters (checkin / proxy-logs). Native datetime inputs render in the
// browser's own locale and offer no "last 24h" affordance, so operators typed
// bounds by hand for the most common windows. One chip row fixes that without
// a bespoke date picker (round-2 audit: custom picker ROI was rejected).

import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

/** `YYYY-MM-DDTHH:mm` in LOCAL time — the datetime-local input's value shape. */
function toDatetimeLocalValue(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    `T${pad(date.getHours())}:${pad(date.getMinutes())}`
  )
}

type Preset = {
  key: 'today' | 'last24Hours' | 'last7Days' | 'last30Days'
  resolve: (now: Date) => [Date, Date]
}

const HOUR = 60 * 60 * 1000
const DAY = 24 * HOUR

const PRESETS: Preset[] = [
  {
    key: 'today',
    resolve: (now) => {
      const start = new Date(now)
      start.setHours(0, 0, 0, 0)
      return [start, now]
    },
  },
  { key: 'last24Hours', resolve: (now) => [new Date(+now - DAY), now] },
  { key: 'last7Days', resolve: (now) => [new Date(+now - 7 * DAY), now] },
  { key: 'last30Days', resolve: (now) => [new Date(+now - 30 * DAY), now] },
]

export type DateRangePresetsProps = {
  /** Called with datetime-local input values for the from/to pair. */
  onApply: (from: string, to: string) => void
}

/** One row of quick range chips; wraps full-width on mobile like the inputs. */
export function DateRangePresets({ onApply }: DateRangePresetsProps) {
  const { t } = useTranslation()
  return (
    <div
      role='group'
      aria-label={t('common.datePresets.label')}
      className='flex flex-wrap items-center gap-1.5 max-sm:w-full'
    >
      {PRESETS.map((preset) => (
        <Button
          key={preset.key}
          type='button'
          variant='outline'
          size='xs'
          onClick={() => {
            const [from, to] = preset.resolve(new Date())
            onApply(toDatetimeLocalValue(from), toDatetimeLocalValue(to))
          }}
        >
          {t(`common.datePresets.${preset.key}`)}
        </Button>
      ))}
    </div>
  )
}
