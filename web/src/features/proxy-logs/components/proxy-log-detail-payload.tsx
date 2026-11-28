import { useTranslation } from 'react-i18next'

import { Separator } from '@/components/ui/separator'

import type { ProxyLogDetail } from '../types'
import { BillingSection } from './billing-section'
import { JsonBlock } from './json-block'

export function DetailPayload({ detail }: { detail: ProxyLogDetail }) {
  const { t } = useTranslation()
  return (
    <>
      {detail.billingDetails ? (
        <BillingSection billing={detail.billingDetails} />
      ) : null}
      <RawBodySection
        title={t('proxyLogs.detail.rawRequest')}
        body={detail.requestBody}
      />
      <RawBodySection
        title={t('proxyLogs.detail.rawResponse')}
        body={detail.responseBody}
      />
      <RawBodySection
        title={t('proxyLogs.detail.rawRequestHeaders')}
        body={detail.requestHeaders}
      />
      <RawBodySection
        title={t('proxyLogs.detail.rawResponseHeaders')}
        body={detail.responseHeaders}
      />
    </>
  )
}

function RawBodySection({
  title,
  body,
}: {
  title: string
  body?: string | null
}) {
  if (!body || !body.trim()) return null
  return (
    <>
      <Separator />
      <section>
        <h3 className='mb-2 text-sm font-medium'>{title}</h3>
        <JsonBlock value={body} />
      </section>
    </>
  )
}
