/* eslint-disable no-nested-ternary -- loading, error, empty and populated attention states */
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Inbox, ShieldCheck, TriangleAlert } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import {
  attentionLabel,
  type AttentionItem,
  type AttentionResponse,
} from '@/lib/attention-label'
import { formatAbsoluteDateTime, formatRelativeTime } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  resolveAttentionTarget,
  type AttentionTargetLocation,
} from './attention-target'

const SEVERITY_TONE: Record<
  'critical' | 'warning' | 'info',
  { dot: string; variant: 'destructive' | 'warning' | 'info' }
> = {
  critical: {
    dot: 'bg-destructive',
    variant: 'destructive',
  },
  warning: {
    dot: 'bg-warning',
    variant: 'warning',
  },
  info: {
    dot: 'bg-info',
    variant: 'info',
  },
}

/**
 * Merge consecutive duplicate items (same category / label / severity /
 * target — e.g. a persistent "All proxies failed" storm pushing one row per
 * scan window) into a single row carrying a ×N count. The row keeps the
 * newest createdAt; distinct events stay separate.
 */
function mergeAttentionItems(
  items: AttentionItem[]
): Array<{ item: AttentionItem; count: number }> {
  const merged: Array<{ item: AttentionItem; count: number }> = []
  for (const item of items) {
    const previous = merged.at(-1)
    if (
      previous &&
      previous.item.category === item.category &&
      previous.item.label === item.label &&
      previous.item.severity === item.severity &&
      previous.item.target === item.target
    ) {
      previous.count += 1
      if (item.createdAt > previous.item.createdAt) previous.item = item
    } else {
      merged.push({ item, count: 1 })
    }
  }
  return merged
}

/**
 * Router-aware rendering of a parsed attention target. The backend target
 * contract is small and pinned by handler tests, so a switch over the parsed
 * discriminant keeps `to` / `search` / `params` fully typed (a union `Link`
 * props object would need a cast). A parsed target is always SPA-navigable:
 * the settings route redirects unknown sections to the subarea default, and
 * the accounts/sites one-shot params resolve against the loaded snapshot.
 */
function AttentionTargetLink({
  location,
  children,
  title,
}: {
  location: AttentionTargetLocation
  children: ReactNode
  /** Full label text — hover tooltip when the row is truncated. */
  title: string
}) {
  switch (location.to) {
    case '/accounts':
      return (
        <Link
          to='/accounts'
          search={{ accountId: location.search.accountId }}
          title={title}
          className='block truncate text-sm hover:underline'
        >
          {children}
        </Link>
      )
    case '/sites':
      return (
        <Link
          to='/sites'
          search={{ edit: location.search.edit }}
          title={title}
          className='block truncate text-sm hover:underline'
        >
          {children}
        </Link>
      )
    case '/settings/$subarea/$section':
      return (
        <Link
          to='/settings/$subarea/$section'
          params={location.params}
          title={title}
          className='block truncate text-sm hover:underline'
        >
          {children}
        </Link>
      )
  }
}

export function AttentionPanel() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const { data, isLoading, isError } = useQuery({
    queryKey: ['dashboard-attention', 20],
    queryFn: () => api.getAttention(20) as Promise<AttentionResponse>,
    // Auto-refresh the attention panel so expired accounts / low balances
    // surface without a manual reload (the realtime QPS panel uses the ops
    // WebSocket stream; this covers the REST half of availability).
    refetchInterval: 10 * 1000,
  })

  const items = data?.items ?? []
  const mergedItems = mergeAttentionItems(items)

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <ShieldCheck className='size-4' />
          {t('dashboard.availability.monitors.title')}
        </CardTitle>
        <CardDescription className='text-xs'>
          {t('dashboard.availability.monitors.description')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className='h-48 w-full rounded-md' />
        ) : isError ? (
          <Empty className='min-h-48 border'>
            <EmptyHeader>
              <EmptyMedia
                variant='icon'
                className='bg-destructive/10 text-destructive-soft-fg'
              >
                <TriangleAlert />
              </EmptyMedia>
              <EmptyDescription className='text-destructive'>
                {t('dashboard.availability.monitors.loadError')}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : items.length === 0 ? (
          <Empty className='min-h-48 border'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <Inbox />
              </EmptyMedia>
              <EmptyDescription>
                {t('dashboard.availability.monitors.empty')}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <ul className='space-y-2'>
            {mergedItems.map(({ item, count }, index) => {
              const tone = SEVERITY_TONE[item.severity] ?? SEVERITY_TONE.info
              const label = t(
                `dashboard.availability.monitors.severity.${item.severity}`
              )
              const itemLabel = attentionLabel(item, t)
              const relativeTime = formatRelativeTime(item.createdAt, locale)
              // Unrecognized targets (backend drift, malformed url) render as
              // plain text — never a dead anchor.
              const targetLocation = item.target
                ? resolveAttentionTarget(item.target)
                : null
              return (
                <li
                  // The API has no stable item ID; position distinguishes repeated summaries.
                  // eslint-disable-next-line react/no-array-index-key
                  key={`${item.target}-${index}`}
                  className='flex items-start gap-3'
                >
                  <div className='mt-1 flex shrink-0 items-center gap-1'>
                    <Badge variant={tone.variant}>
                      <span
                        aria-hidden='true'
                        className={cn('size-1.5 rounded-full', tone.dot)}
                      />
                      {label}
                    </Badge>
                    {count > 1 ? (
                      <Badge
                        variant='outline'
                        title={t(
                          'dashboard.availability.monitors.mergedCount',
                          { count }
                        )}
                      >
                        ×{count}
                      </Badge>
                    ) : null}
                  </div>
                  <div className='min-w-0 flex-1'>
                    {targetLocation ? (
                      <AttentionTargetLink
                        location={targetLocation}
                        title={itemLabel}
                      >
                        {itemLabel}
                      </AttentionTargetLink>
                    ) : (
                      <span
                        className='block truncate text-sm'
                        title={itemLabel}
                      >
                        {itemLabel}
                      </span>
                    )}
                  </div>
                  {relativeTime ? (
                    <time
                      dateTime={item.createdAt}
                      title={formatAbsoluteDateTime(item.createdAt, locale)}
                      className='text-muted-foreground mt-0.5 shrink-0 text-xs tabular-nums'
                    >
                      {relativeTime}
                    </time>
                  ) : null}
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
