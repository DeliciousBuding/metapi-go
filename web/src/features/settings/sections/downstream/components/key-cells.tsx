// metapi-go/features/settings/sections/downstream — downstream-key table
// cell renderers (model policy summary badge, quota + 24h usage). Split out
// of keys-section.tsx (S8 giant-file teardown); behavior is unchanged.
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import { parseIdArray } from '../lib/credential-refs'
import {
  normalizeModelRules,
  type DownstreamApiKeyItem,
} from './key-form-shared'

export function KeyModelPolicyCell({
  supportedModels,
  allowedRouteIds,
}: {
  supportedModels?: DownstreamApiKeyItem['supportedModels']
  allowedRouteIds?: DownstreamApiKeyItem['allowedRouteIds']
}) {
  const { t } = useTranslation()
  const rules = normalizeModelRules(supportedModels)
  const routeGrantCount = parseIdArray(allowedRouteIds).length
  if (rules.includes('*')) {
    return (
      <Badge variant='success'>
        {t('settings.downstream.keys.models.summary.all', {
          defaultValue: 'All models',
        })}
      </Badge>
    )
  }
  if (rules.length === 0 && routeGrantCount > 0) {
    return (
      <Badge variant='outline'>
        {t('settings.downstream.keys.models.summary.routes', {
          count: routeGrantCount,
          defaultValue: '{{count}} route grants',
        })}
      </Badge>
    )
  }
  if (rules.length === 0) {
    return (
      <Badge variant='warning'>
        {t('settings.downstream.keys.models.summary.none', {
          defaultValue: 'No models authorized',
        })}
      </Badge>
    )
  }
  return (
    <Badge variant='outline'>
      {t('settings.downstream.keys.models.summary.rules', {
        count: rules.length,
        defaultValue: '{{count}} rules',
      })}
    </Badge>
  )
}

// Exported so the usage-cell test can render it in isolation (same pattern as
// KeySheetForm). Renders quota usage plus the per-key 24h proxy_logs summary.
export function KeyUsageCell({ item }: { item: DownstreamApiKeyItem }) {
  const { t } = useTranslation()
  return (
    <div>
      <div>
        {t('settings.downstream.keys.requests', {
          used: item.usedRequests ?? 0,
          max: item.maxRequests ?? t('settings.common.unlimited'),
        })}
      </div>
      <div>
        {t('settings.downstream.keys.cost', {
          used: item.usedCost ?? 0,
          max: item.maxCost ?? t('settings.common.unlimited'),
        })}
      </div>
      <div className='mt-1 border-t pt-1'>
        {t('settings.downstream.keys.usage24h', {
          requests: item.usage24h?.requests ?? 0,
          tokens: item.usage24h?.tokens ?? 0,
          cost: item.usage24h?.cost ?? 0,
        })}
      </div>
    </div>
  )
}
