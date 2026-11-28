import {
  FlaskConical as FlaskConicalIcon,
  RefreshCw as RefreshCwIcon,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

import { supplyKindLabels } from '../lib/supply-kind-labels'
import type { ModelsMarketplaceFacets } from '../types'

export function useModelsToolbar(
  facets: ModelsMarketplaceFacets | null,
  onVerify: () => void,
  onRefresh: () => void,
  refreshing: boolean
) {
  const { t } = useTranslation()
  // Facet options arrive with the page response and therefore cover the whole
  // marketplace; each dimension's counts are disjunctive (computed with its
  // own filter removed), so selecting a value never hides the alternatives.
  const capabilityFilterOptions = useMemo(
    () =>
      (facets?.capabilities ?? []).map((facet) => ({
        label: facet.value,
        value: facet.value,
        count: facet.count,
      })),
    [facets]
  )
  const endpointTypeFilterOptions = useMemo(
    () =>
      (facets?.endpointTypes ?? []).map((facet) => ({
        label: facet.value,
        value: facet.value,
        count: facet.count,
      })),
    [facets]
  )
  const sourceKindFilterOptions = useMemo(
    () =>
      (facets?.sourceKinds ?? []).map((facet) => ({
        label: supplyKindLabels[facet.value as keyof typeof supplyKindLabels]
          ? t(supplyKindLabels[facet.value as keyof typeof supplyKindLabels])
          : facet.value,
        value: facet.value,
        count: facet.count,
      })),
    [facets, t]
  )

  return {
    searchPlaceholder: t('models.toolbar.searchPlaceholder'),
    searchDebounceMs: 400,
    filters: [
      {
        columnId: 'sourceKinds',
        title: t('models.filters.sourceKind'),
        options: sourceKindFilterOptions,
      },
      {
        columnId: 'endpointTypes',
        title: t('models.columns.endpointTypes'),
        options: endpointTypeFilterOptions,
      },
      {
        columnId: 'capabilities',
        title: t('models.columns.capabilities'),
        options: capabilityFilterOptions,
      },
    ],
    preActions: (
      <>
        <Button variant='outline' size='sm' onClick={onVerify}>
          <FlaskConicalIcon className='size-3.5' />
          {t('models.toolbar.batchProbe')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          onClick={onRefresh}
          disabled={refreshing}
        >
          {refreshing ? <Spinner /> : <RefreshCwIcon className='size-3.5' />}
          {t('models.toolbar.refresh')}
        </Button>
      </>
    ),
  }
}
