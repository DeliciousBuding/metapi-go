import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import {
  isKeyRestricted,
  KEY_RESTRICTION_DIMENSIONS,
  keyRestrictionDimensions,
  type KeyRestrictionDimension,
} from '../lib/key-restrictions'
import type { DownstreamApiKeyItem } from '../types'

// Stable empty list so the memos below do not re-run on every render when
// the query has no data yet.
const EMPTY_KEY_ITEMS: DownstreamApiKeyItem[] = []

export function useKeyRestrictionFilter(
  allItems: DownstreamApiKeyItem[] = EMPTY_KEY_ITEMS
) {
  const { t } = useTranslation()
  // 'all' | 'restricted' | 'unrestricted' | `dim:<dimension>` (#1430
  // per-dimension filter for "which keys touch IP lists" style audits).
  const [restrictionFilter, setRestrictionFilter] = useState<string>('all')
  const restrictedCount = useMemo(
    () => allItems.filter(isKeyRestricted).length,
    [allItems]
  )
  const items = useMemo(() => {
    if (restrictionFilter === 'restricted') {
      return allItems.filter(isKeyRestricted)
    }
    if (restrictionFilter === 'unrestricted') {
      return allItems.filter((item) => !isKeyRestricted(item))
    }
    if (restrictionFilter.startsWith('dim:')) {
      const dimension = restrictionFilter.slice(4) as KeyRestrictionDimension
      return allItems.filter((item) =>
        keyRestrictionDimensions(item).includes(dimension)
      )
    }
    return allItems
  }, [allItems, restrictionFilter])
  const restrictionFilterItems = useMemo(
    () => [
      {
        value: 'all',
        label: t('settings.downstream.keys.restriction.filterAll'),
      },
      {
        value: 'restricted',
        label: t('settings.downstream.keys.restriction.filterRestricted'),
      },
      {
        value: 'unrestricted',
        label: t('settings.downstream.keys.restriction.filterUnrestricted'),
      },
      ...KEY_RESTRICTION_DIMENSIONS.map((dimension) => ({
        value: `dim:${dimension}`,
        label: t(`settings.downstream.keys.restriction.dim.${dimension}`),
      })),
    ],
    [t]
  )
  return {
    items,
    toolbar: (
      <>
        <Select
          items={restrictionFilterItems}
          value={restrictionFilter}
          onValueChange={(value) => setRestrictionFilter(value ?? 'all')}
        >
          <SelectTrigger
            size='sm'
            aria-label={t('settings.downstream.keys.restriction.filterAria')}
            className='w-44'
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {restrictionFilterItems.map(({ value, label }) => (
              <SelectItem key={value} value={value}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className='text-muted-foreground self-center text-xs'>
          {t('settings.downstream.keys.restriction.count', {
            restricted: restrictedCount,
            total: allItems.length,
          })}
        </span>
      </>
    ),
  }
}
