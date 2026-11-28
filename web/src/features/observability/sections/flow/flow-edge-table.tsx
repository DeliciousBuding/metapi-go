import { useTranslation } from 'react-i18next'

import type { TrafficFlowEdge } from '@/lib/api'
import { formatCurrency, formatInt } from '@/lib/format'

export function FlowEdgeTable({
  edges,
  locale,
}: {
  edges: TrafficFlowEdge[]
  locale: string
}) {
  const { t } = useTranslation()
  return (
    <table className='sr-only'>
      <caption>{t('observability.flow.cardTitle')}</caption>
      <thead>
        <tr>
          <th scope='col'>{t('observability.flow.columns.key')}</th>
          <th scope='col'>{t('observability.flow.columns.model')}</th>
          <th scope='col'>{t('observability.flow.columns.upstream')}</th>
          <th scope='col'>{t('observability.flow.summary.requests')}</th>
          <th scope='col'>{t('observability.flow.summary.failed')}</th>
          <th scope='col'>{t('observability.flow.summary.tokens')}</th>
          <th scope='col'>{t('observability.flow.summary.cost')}</th>
        </tr>
      </thead>
      <tbody>
        {edges.map((edge) => (
          <tr
            key={`${edge.downstreamKeyId ?? 'none'}:${edge.downstreamKeyName ?? ''}:${edge.model}:${edge.siteId ?? 'none'}:${edge.siteName ?? ''}:${edge.channelId ?? 'none'}`}
          >
            <td>
              {edge.downstreamKeyName ||
                (edge.downstreamKeyId
                  ? `#${edge.downstreamKeyId}`
                  : t('observability.flow.unassignedKey'))}
            </td>
            <td>{edge.model || t('observability.flow.unknownModel')}</td>
            <td>
              {edge.siteName ||
                (edge.siteId
                  ? `#${edge.siteId}`
                  : t('observability.flow.unassignedUpstream'))}
              {edge.channelId ? ` (#${edge.channelId})` : ''}
            </td>
            <td>{formatInt(edge.requests, locale)}</td>
            <td>{formatInt(edge.failedCount, locale)}</td>
            <td>{formatInt(edge.totalTokens, locale)}</td>
            <td>
              {formatCurrency(edge.totalCost, {
                fractionDigits: 4,
                locale,
              })}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
