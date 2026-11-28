import { useTranslation } from 'react-i18next'

import { ExpandableText } from '@/components/common/expandable-text'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { formatAbsoluteDateTime, formatRelativeTime } from '@/lib/format'

import type { SiteApiEndpoint } from '../types'

export function SiteDetailEndpoints({
  endpoints,
  locale,
}: {
  endpoints: SiteApiEndpoint[]
  locale: string
}) {
  const { t } = useTranslation()
  if (!endpoints.length) return null
  return (
    <>
      <Separator />
      <section>
        <h3 className='text-sm font-medium'>{t('sites.detail.endpoints')}</h3>
        <ul className='mt-2 space-y-2'>
          {endpoints.map((endpoint) => (
            <EndpointRow
              key={endpoint.url}
              endpoint={endpoint}
              locale={locale}
            />
          ))}
        </ul>
      </section>
    </>
  )
}

function EndpointRow({
  endpoint,
  locale,
}: {
  endpoint: SiteApiEndpoint
  locale: string
}) {
  const { t } = useTranslation()
  const cooldownActive =
    Boolean(endpoint.cooldownUntil) &&
    new Date(endpoint.cooldownUntil as string) > new Date()
  const failureReason = endpoint.lastFailureReason?.trim() || null
  return (
    <li className='flex flex-col gap-1 text-sm'>
      <div className='flex items-start gap-2'>
        <Badge variant={endpoint.enabled === false ? 'secondary' : 'outline'}>
          {endpoint.enabled === false
            ? t('sites.detail.disabled')
            : t('sites.detail.enabled')}
        </Badge>
        <ExpandableText
          text={endpoint.url}
          className='min-w-0 flex-1 text-sm leading-normal'
        >
          <span className='break-all'>{endpoint.url}</span>
        </ExpandableText>
      </div>
      {cooldownActive && (
        <div className='flex items-center gap-1.5'>
          <Badge variant='warning'>{t('sites.detail.endpointCooldown')}</Badge>
          <span
            className='text-muted-foreground text-xs tabular-nums'
            title={
              formatAbsoluteDateTime(endpoint.cooldownUntil, locale) ||
              undefined
            }
          >
            {formatRelativeTime(endpoint.cooldownUntil, locale)}
          </span>
        </div>
      )}
      {failureReason && (
        <div>
          <div className='text-muted-foreground text-xs font-medium'>
            {t('sites.detail.endpointFailureReason')}
          </div>
          <ExpandableText
            text={failureReason}
            className='text-destructive text-sm leading-normal break-words'
          />
        </div>
      )}
    </li>
  )
}
