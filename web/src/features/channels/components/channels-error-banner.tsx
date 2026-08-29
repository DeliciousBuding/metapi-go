// metapi-go/features/channels — error-count banner that doubles as a
// one-click filter entry (competitor-study-2026-08 P1-4: axonhub turns the
// "N failing" banner into the filter entry so the error state and the action
// state are one surface). When the loaded list contains failing channels,
// the banner counts them and offers a single action that narrows the table
// to exactly those rows; once the URL filter is scoped to failing statuses
// it switches to a clearable "error-only" indicator with an exit action.
//
// URL semantics follow docs/internal/design/state-stability.md R1: the filter
// is URL-owned persistent state (the existing shareable `?status=` facet),
// not a one-shot deep-link param. One-shot consumption (the `?channelId=`
// drilldown pattern) is reserved for transient UI such as dialogs and
// sheets; a filter must stay shareable, survive back/forward, and remain
// clearable through the toolbar facet. The banner therefore never strips
// the param — it derives its mode from it.

import { Filter, TriangleAlert, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'

type ChannelsErrorBannerProps = {
  /** Number of failing channels in the loaded list; renders nothing at 0. */
  errorCount: number
  /** True when the URL filter already scopes the view to failing channels. */
  showErrorOnly: boolean
  /** Narrows the table to the failing channels (single URL transaction). */
  onFilterErrors: () => void
  /** Clears the error-only filter back to the full list. */
  onExitErrorOnly: () => void
}

export function ChannelsErrorBanner({
  errorCount,
  showErrorOnly,
  onFilterErrors,
  onExitErrorOnly,
}: ChannelsErrorBannerProps) {
  const { t } = useTranslation()
  if (errorCount === 0) return null

  return (
    <Notice
      tone='warning'
      role='alert'
      className='items-center justify-between gap-3'
    >
      <div className='flex items-start gap-2'>
        <TriangleAlert className='mt-0.5 size-4 shrink-0' />
        <span>
          {showErrorOnly
            ? t('channels.errorBanner.errorOnlyMode')
            : t('channels.errorBanner.message', { count: errorCount })}
        </span>
      </div>
      {showErrorOnly ? (
        <Button variant='outline' size='sm' onClick={onExitErrorOnly}>
          <X className='size-3.5' />
          {t('channels.errorBanner.exitButton')}
        </Button>
      ) : (
        <Button variant='outline' size='sm' onClick={onFilterErrors}>
          <Filter className='size-3.5' />
          {t('channels.errorBanner.filterButton')}
        </Button>
      )}
    </Notice>
  )
}
