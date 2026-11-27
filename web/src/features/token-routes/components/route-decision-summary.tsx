import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime } from '@/lib/format'

import type { RouteDecision } from '../types'
import { routingStrategyLabel } from '../utils'

export function RouteDecisionSummary({
  decision,
  snapshotAt,
}: {
  decision: RouteDecision
  snapshotAt?: string | null
}) {
  const { t, i18n } = useTranslation()
  const checkedAt = snapshotAt ?? decision.checkedAt ?? decision.generatedAt
  const model = decision.requestedModel ?? decision.model
  const pattern = decision.modelPattern ?? decision.matchedRoutePattern
  return (
    <>
      <dl className='grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1'>
        <dt className='text-muted-foreground'>
          {t('tokenRoutes.detail.decisionModel')}
        </dt>
        <dd className='min-w-0'>{model ? <ModelPill model={model} /> : '—'}</dd>
        <dt className='text-muted-foreground'>
          {t('tokenRoutes.preview.checkedAt')}
        </dt>
        <dd>{formatDateTime(checkedAt, toBcp47(i18n.language))}</dd>
        <dt className='text-muted-foreground'>
          {t('tokenRoutes.preview.matchedRoute')}
        </dt>
        <dd className='break-words'>
          {pattern || t('tokenRoutes.preview.noMatch')}
        </dd>
        <dt className='text-muted-foreground'>
          {t('tokenRoutes.preview.strategy')}
        </dt>
        <dd>
          {decision.routingStrategy
            ? t(routingStrategyLabel(decision.routingStrategy))
            : '—'}
        </dd>
      </dl>
      <p className='text-muted-foreground'>
        {t('tokenRoutes.preview.noSelection')}
      </p>
      {!decision.requiredUpstreamProtocol && (
        <p>{t('tokenRoutes.preview.protocolUnknown')}</p>
      )}
      {decision.allowUpstreamProtocolConversion && (
        <p>{t('tokenRoutes.preview.conversion')}</p>
      )}
      {decision.notEvaluated?.includes('concreteModel') && (
        <p>{t('tokenRoutes.preview.modelUnknown')}</p>
      )}
    </>
  )
}
