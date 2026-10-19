import { useTranslation } from 'react-i18next'

import { toBcp47 } from '@/i18n/languages'
import { formatInt } from '@/lib/format'

import type { ProxyLog } from '../types'

export function UsageCell({
  log,
}: {
  log: Pick<
    ProxyLog,
    | 'promptTokens'
    | 'completionTokens'
    | 'cacheReadTokens'
    | 'cacheCreationTokens'
  >
}) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const count = (value: number | null | undefined) =>
    value != null && value >= 0 ? formatInt(value, locale) : '—'
  const cacheRead =
    typeof log.cacheReadTokens === 'number' &&
    Number.isFinite(log.cacheReadTokens) &&
    log.cacheReadTokens > 0
      ? count(log.cacheReadTokens)
      : null
  const cacheCreation =
    typeof log.cacheCreationTokens === 'number' &&
    Number.isFinite(log.cacheCreationTokens) &&
    log.cacheCreationTokens > 0
      ? count(log.cacheCreationTokens)
      : null
  const detail = [
    `${t('proxyLogs.usage.input')} ${count(log.promptTokens)}`,
    `${t('proxyLogs.usage.output')} ${count(log.completionTokens)}`,
    cacheRead && `${t('proxyLogs.usage.cacheRead')} ${cacheRead}`,
    cacheCreation && `${t('proxyLogs.usage.cacheCreation')} ${cacheCreation}`,
  ]
    .filter(Boolean)
    .join(' · ')

  return (
    <div
      className='flex min-w-0 items-baseline gap-1.5 text-xs tabular-nums'
      title={detail}
    >
      <span className='whitespace-nowrap'>
        {count(log.promptTokens)}
        <span className='text-muted-foreground'> / </span>
        {count(log.completionTokens)}
      </span>
      {cacheRead && (
        <span className='text-muted-foreground text-2xs whitespace-nowrap'>
          <span className='sr-only'>{t('proxyLogs.usage.cacheRead')} </span>+
          {cacheRead}
        </span>
      )}
    </div>
  )
}
