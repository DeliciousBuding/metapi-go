import { CircleSlash2, Layers3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
import { RowDisclosureTrigger } from '@/components/common/row-disclosure'

import type { RouteSummaryRow } from '../types'
import { isExplicitGroupRoute, resolveRouteTitle } from '../utils'
import { RoutesModelSummary } from './routes-model-summary'

export type RouteDisclosure = {
  expandedIds: ReadonlySet<number>
  toggle: (id: number) => void
  getContentId: (id: number) => string
}
export function RoutesTitleCell({
  route,
  readOnly,
  disclosure,
}: {
  route: RouteSummaryRow
  readOnly: boolean
  disclosure?: RouteDisclosure
}) {
  const { t } = useTranslation()
  const title = resolveRouteTitle(route)
  const group = isExplicitGroupRoute(route)
  const modeLabel = t(
    group ? 'tokenRoutes.columns.badgeGroup' : 'tokenRoutes.columns.badgeMatch'
  )
  return (
    <div className='flex min-w-0 items-center gap-2.5 py-0.5'>
      <span className='bg-background dark:bg-muted grid size-8 shrink-0 place-items-center rounded-lg shadow-sm ring-1 ring-black/8 dark:ring-white/12'>
        <BrandGlyph model={title} icon={route.displayIcon} size={22} alt='' />
      </span>
      <div className='min-w-0'>
        <div className='flex min-w-0 items-center gap-1.5'>
          {disclosure ? (
            <RowDisclosureTrigger
              expanded={disclosure.expandedIds.has(route.id)}
              controls={disclosure.getContentId(route.id)}
              onClick={() => disclosure.toggle(route.id)}
              title={title}
              className='min-h-0 gap-1 px-0 text-sm font-medium hover:bg-transparent'
            >
              <span className='min-w-0 truncate'>{title}</span>
            </RowDisclosureTrigger>
          ) : (
            <span className='max-w-60 truncate text-sm font-medium'>
              {title}
            </span>
          )}
          {group ? (
            <span
              className='text-muted-foreground inline-flex shrink-0'
              title={modeLabel}
            >
              <Layers3 className='size-3.5' aria-hidden='true' />
              <span className='sr-only'>{modeLabel}</span>
            </span>
          ) : (
            <span className='sr-only'>{modeLabel}</span>
          )}
          {readOnly && (
            <span
              className='text-muted-foreground inline-flex shrink-0'
              title={t('tokenRoutes.columns.notGenerated')}
            >
              <CircleSlash2 className='size-3.5' aria-hidden='true' />
              <span className='sr-only'>
                {t('tokenRoutes.columns.notGenerated')}
              </span>
            </span>
          )}
        </div>
        <RoutesModelSummary route={route} readOnly={readOnly} />
      </div>
    </div>
  )
}
