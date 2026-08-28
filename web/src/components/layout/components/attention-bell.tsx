// metapi-go/layout — global attention bell.
//
// Audit follow-up (#887): actionable attention items were only visible on the
// dashboard (today snapshot + availability panel), so an operator inspecting
// any other page had no way to notice a new critical item, let alone jump back
// to the problem. The header now carries a bell with a severity-toned count
// badge plus a popover listing the most recent items, each deep-linking to the
// page that resolves it.
//
// The data source is the dashboard's existing attention query — same
// `['dashboard-attention', 20]` key, same `api.getAttention(20)` wrapper —
// so TanStack Query dedupes the request and shares one cache entry instead of
// opening a second data source. The poll is slower than the availability
// panel's (15s vs 10s): the header badge tolerates lag, and while the panel is
// open its 10s poll keeps the shared entry fresh anyway.

import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { Bell } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import { formatAbsoluteDateTime, formatRelativeTime } from '@/lib/format'
import { cn } from '@/lib/utils'

/** Matches the dashboard attention query so the cache entry is shared. */
const ATTENTION_LIMIT = 20
/** Slower than the availability panel's 10s poll — the badge tolerates lag. */
const ATTENTION_REFETCH_INTERVAL_MS = 15 * 1000
/** Counts above this render as "9+" so the badge never widens the trigger. */
const BADGE_COUNT_CEILING = 9
/** The popover is a peek surface; "view all" leads to the full panel. */
const POPOVER_ITEM_LIMIT = 6

type AttentionSeverity = 'critical' | 'warning' | 'info'

type AttentionItem = {
  severity: AttentionSeverity
  category: string
  label: string
  target: string
  createdAt: string
  params?: Record<string, string | number>
}

/** Attention response (GET /api/stats/attention?limit=20). */
type AttentionResponse = {
  items: AttentionItem[]
  total: number
}

const SEVERITY_BADGE_VARIANT: Record<
  AttentionSeverity,
  'destructive' | 'warning' | 'info'
> = {
  critical: 'destructive',
  warning: 'warning',
  info: 'info',
}

const SEVERITY_DOT_CLASS: Record<AttentionSeverity, string> = {
  critical: 'bg-destructive',
  warning: 'bg-warning',
  info: 'bg-info',
}

/** Solid tone for the trigger's count badge (the "red dot" affordance). */
const SEVERITY_INDICATOR_CLASS: Record<AttentionSeverity, string> = {
  critical: 'bg-destructive text-destructive-foreground',
  warning: 'bg-warning text-warning-foreground',
  info: 'bg-info text-info-foreground',
}

/** Most severe first — the badge tone follows the worst pending item. */
const SEVERITY_PRIORITY: AttentionSeverity[] = ['critical', 'warning', 'info']

/**
 * The worst severity present, or `null` when nothing is pending (no items →
 * no red dot). Items carrying a severity the UI does not know about still
 * count, and read as `info`.
 */
function resolveHighestSeverity(
  items: AttentionItem[]
): AttentionSeverity | null {
  if (items.length === 0) return null
  for (const severity of SEVERITY_PRIORITY) {
    if (items.some((item) => item.severity === severity)) return severity
  }
  return 'info'
}

function formatBadgeCount(count: number): string {
  return count > BADGE_COUNT_CEILING ? `${BADGE_COUNT_CEILING}+` : String(count)
}

type AttentionListProps = {
  items: AttentionItem[]
  locale: string
  onSelect: (item: AttentionItem) => void
}

function AttentionList(props: AttentionListProps) {
  const { t } = useTranslation()

  return (
    <ul className='flex flex-col'>
      {props.items.map((item, index) => {
        const badgeVariant =
          SEVERITY_BADGE_VARIANT[item.severity] ?? SEVERITY_BADGE_VARIANT.info
        const dotClass =
          SEVERITY_DOT_CLASS[item.severity] ?? SEVERITY_DOT_CLASS.info
        const relativeTime = formatRelativeTime(item.createdAt, props.locale)
        return (
          <li
            // The backend has no stable item id — target + position is unique
            // within one severity-ranked response.
            // eslint-disable-next-line react/no-array-index-key
            key={`${item.target}-${index}`}
          >
            <Button
              variant='ghost'
              className='h-auto w-full items-start justify-start gap-2 px-2 py-2 text-start font-normal whitespace-normal'
              onClick={() => props.onSelect(item)}
            >
              <Badge variant={badgeVariant} className='mt-px shrink-0'>
                <span
                  aria-hidden='true'
                  className={cn('size-1.5 rounded-full', dotClass)}
                />
                {t(`attention.severity.${item.severity}`)}
              </Badge>
              <span className='min-w-0 flex-1 truncate text-sm'>
                {item.label}
              </span>
              {relativeTime ? (
                <time
                  dateTime={item.createdAt}
                  title={formatAbsoluteDateTime(item.createdAt, props.locale)}
                  className='text-muted-foreground shrink-0 text-xs tabular-nums'
                >
                  {relativeTime}
                </time>
              ) : null}
            </Button>
          </li>
        )
      })}
    </ul>
  )
}

export function AttentionBell() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)

  const { data, isLoading, isError } = useQuery({
    queryKey: ['dashboard-attention', ATTENTION_LIMIT],
    queryFn: () =>
      api.getAttention(ATTENTION_LIMIT) as Promise<AttentionResponse>,
    refetchInterval: ATTENTION_REFETCH_INTERVAL_MS,
  })

  const items = data?.items ?? []
  const highestSeverity = resolveHighestSeverity(items)

  const handleSelect = (item: AttentionItem) => {
    setOpen(false)

    let idKey: 'announcementId' | 'eventId' | null = null
    if (item.category === 'site_announcement') idKey = 'announcementId'
    else if (item.category === 'event') idKey = 'eventId'
    const rawID = idKey ? item.params?.[idKey] : undefined
    const itemID = typeof rawID === 'number' ? rawID : Number(rawID)
    const hasValidID = Number.isInteger(itemID) && itemID > 0
    let acknowledge: (() => Promise<unknown>) | null = null
    if (idKey === 'announcementId' && hasValidID) {
      acknowledge = () => api.markSiteAnnouncementRead(itemID)
    } else if (idKey === 'eventId' && hasValidID) {
      acknowledge = () => api.markEventRead(itemID)
    }

    if (acknowledge) {
      // The read owner lives in the announcement/event table. Remove the row
      // optimistically so the header count reacts to the click, then refetch
      // to restore it if the write fails. Computed account/site conditions
      // intentionally stay until their underlying condition is resolved.
      queryClient.setQueryData<AttentionResponse>(
        ['dashboard-attention', ATTENTION_LIMIT],
        (current) =>
          current
            ? {
                ...current,
                items: current.items.filter((candidate) => candidate !== item),
                total: Math.max(0, current.total - 1),
              }
            : current
      )
      void acknowledge().finally(() => {
        void queryClient.invalidateQueries({
          queryKey: ['dashboard-attention'],
        })
      })
    }

    // `target` is a plain SPA path (possibly with a query string), so it goes
    // through the router's `href` option instead of window.location.
    if (item.target) navigate({ href: item.target })
  }

  const triggerLabel =
    items.length > 0
      ? t('attention.triggerWithCount', { value: items.length })
      : t('attention.trigger')

  const renderPopoverBody = (): ReactNode => {
    if (isLoading) {
      return (
        <div className='flex flex-col gap-2 p-2'>
          <Skeleton className='h-5 w-full' />
          <Skeleton className='h-5 w-4/5' />
          <Skeleton className='h-5 w-3/5' />
        </div>
      )
    }
    if (isError) {
      return (
        <p className='text-destructive px-2 py-6 text-center text-sm'>
          {t('attention.loadError')}
        </p>
      )
    }
    if (items.length === 0) {
      return (
        <p className='text-muted-foreground px-2 py-6 text-center text-sm'>
          {t('attention.empty')}
        </p>
      )
    }
    return (
      <AttentionList
        items={items.slice(0, POPOVER_ITEM_LIMIT)}
        locale={locale}
        onSelect={handleSelect}
      />
    )
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            className='relative'
            aria-label={triggerLabel}
          />
        }
      >
        <Bell className='size-4' aria-hidden='true' />
        {highestSeverity ? (
          <span
            data-slot='attention-indicator'
            data-severity={highestSeverity}
            aria-hidden='true'
            className={cn(
              'absolute -end-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-[0.625rem] leading-none font-semibold tabular-nums',
              SEVERITY_INDICATOR_CLASS[highestSeverity]
            )}
          >
            {formatBadgeCount(items.length)}
          </span>
        ) : null}
      </PopoverTrigger>
      <PopoverContent
        align='end'
        className='w-80 max-w-(--available-width) gap-0 p-0'
      >
        <div className='flex items-start justify-between gap-2 px-3 py-2'>
          <div className='min-w-0'>
            <p className='text-sm font-semibold'>{t('attention.title')}</p>
            <p className='text-muted-foreground text-xs'>
              {t('attention.description')}
            </p>
          </div>
          {highestSeverity ? (
            <Badge variant={SEVERITY_BADGE_VARIANT[highestSeverity]}>
              {items.length}
            </Badge>
          ) : null}
        </div>
        <Separator />
        <div className='max-h-72 overflow-y-auto p-1'>
          {renderPopoverBody()}
        </div>
        <Separator />
        <Link
          to='/dashboard/$section'
          params={{ section: 'availability' }}
          onClick={() => setOpen(false)}
          className='text-primary block px-3 py-2 text-center text-xs font-medium hover:underline'
        >
          {t('attention.viewAll')}
        </Link>
      </PopoverContent>
    </Popover>
  )
}
