import { useTranslation } from 'react-i18next'

import { toBcp47 } from '@/i18n/languages'
import { formatInt } from '@/lib/format'

import type { ProxyLog } from '../types'

export function UsageCell({ log }: { log: ProxyLog }) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const count = (value: number | null | undefined) =>
    value != null && value >= 0 ? formatInt(value, locale) : '—'
  return (
    <div className='flex flex-col gap-0.5 text-xs tabular-nums'>
      <span className='flex items-baseline gap-2'>
        <span className='text-muted-foreground'>
          {t('proxyLogs.usage.input')}
        </span>
        <span>{count(log.promptTokens)}</span>
      </span>
      <span className='flex items-baseline gap-2'>
        <span className='text-muted-foreground'>
          {t('proxyLogs.usage.output')}
        </span>
        <span>{count(log.completionTokens)}</span>
      </span>
      {typeof log.cacheReadTokens === 'number' &&
      Number.isFinite(log.cacheReadTokens) &&
      log.cacheReadTokens > 0 ? (
        <span className='text-muted-foreground flex items-baseline gap-2'>
          <span>{t('proxyLogs.usage.cacheRead')}</span>
          <span>{count(log.cacheReadTokens)}</span>
        </span>
      ) : null}
      {typeof log.cacheCreationTokens === 'number' &&
      Number.isFinite(log.cacheCreationTokens) &&
      log.cacheCreationTokens > 0 ? (
        <span className='text-muted-foreground flex items-baseline gap-2'>
          <span>{t('proxyLogs.usage.cacheCreation')}</span>
          <span>{count(log.cacheCreationTokens)}</span>
        </span>
      ) : null}
    </div>
  )
}
