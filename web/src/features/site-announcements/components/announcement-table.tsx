import { ExternalLink } from 'lucide-react'
import { Fragment, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  RowDisclosureTrigger,
  RowDisclosureContent,
  rowDisclosureClick,
} from '@/components/common/row-disclosure'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { Site } from '@/features/sites'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'

import { resolveAnnouncementSourceURL } from '../announcement-source-url'
import type { SiteAnnouncement } from '../types'
import { AnnouncementDetails } from './announcement-details'

type BadgeVariant =
  | 'default'
  | 'secondary'
  | 'destructive'
  | 'warning'
  | 'success'
  | 'info'
  | 'outline'

/** Static level → badge variant map (unknown levels render as outline). */
const LEVEL_BADGE_VARIANT: Record<string, BadgeVariant> = {
  info: 'secondary',
  warning: 'warning',
  error: 'destructive',
  critical: 'destructive',
  success: 'success',
}

/** Static level → i18n label key map; avoids dynamic t() interpolation. */
const LEVEL_LABEL_KEY: Record<string, string> = {
  info: 'siteAnnouncements.level.info',
  warning: 'siteAnnouncements.level.warning',
  error: 'siteAnnouncements.level.error',
  critical: 'siteAnnouncements.level.critical',
  success: 'siteAnnouncements.level.success',
}

function LevelBadge({ level }: { level: string | null | undefined }) {
  const { t } = useTranslation()
  const normalized = (level ?? '').trim().toLowerCase()
  const labelKey = LEVEL_LABEL_KEY[normalized]
  return (
    <Badge variant={LEVEL_BADGE_VARIANT[normalized] ?? 'outline'}>
      {labelKey ? t(labelKey) : level || '—'}
    </Badge>
  )
}

/** Row lifecycle hint: dismissed and expired rows get a quiet extra badge. */
function rowLifecycle(item: SiteAnnouncement): 'dismissed' | 'expired' | null {
  if (item.dismissedAt) return 'dismissed'
  if (item.endsAt) {
    const end = Date.parse(item.endsAt)
    if (Number.isFinite(end) && end < Date.now()) return 'expired'
  }
  return null
}

function SeenRange({
  item,
  locale,
}: {
  item: SiteAnnouncement
  locale: string
}) {
  const first = formatDateTime(item.firstSeenAt, locale)
  const last = formatDateTime(item.lastSeenAt, locale)
  if (!last || last === first) return <span>{first}</span>
  return (
    <span className='flex flex-col'>
      <span>{first}</span>
      <span>→ {last}</span>
    </span>
  )
}

export function AnnouncementTable({
  items,
  sites,
  markRead,
  markingRead,
}: {
  items: SiteAnnouncement[]
  sites: Site[] | undefined
  markRead: (id: number) => void
  markingRead: boolean
}) {
  const [expandedIds, setExpandedIds] = useState<Set<number>>(() => new Set())
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const siteById = useMemo(() => {
    const map = new Map<number, Site>()
    for (const site of sites ?? []) {
      map.set(site.id, site)
    }
    return map
  }, [sites])
  if (items.length === 0) return null
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('siteAnnouncements.columns.announcement')}</TableHead>
          <TableHead className='hidden md:table-cell'>
            {t('siteAnnouncements.columns.site')}
          </TableHead>
          <TableHead className='hidden md:table-cell'>
            {t('siteAnnouncements.columns.level')}
          </TableHead>
          <TableHead className='hidden md:table-cell'>
            {t('siteAnnouncements.columns.seen')}
          </TableHead>
          <TableHead className='w-20 text-right md:w-auto'>
            {t('siteAnnouncements.columns.actions')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => {
          const unread = !item.readAt
          const expanded = expandedIds.has(item.id)
          const contentId = `announcement-details-${item.id}`
          const title =
            item.title === 'Site notice'
              ? t('siteAnnouncements.timeline.siteNotice')
              : item.title
          const toggle = () => {
            setExpandedIds((previous) => {
              const next = new Set(previous)
              if (next.has(item.id)) next.delete(item.id)
              else next.add(item.id)
              return next
            })
            // Keep an unread-filtered row visible while its body is open.
            if (expanded && unread) markRead(item.id)
          }
          const lifecycle = rowLifecycle(item)
          const sourceURL = resolveAnnouncementSourceURL(
            item,
            siteById.get(item.siteId)
          )
          return (
            <Fragment key={item.id}>
              <TableRow
                data-unread={unread || undefined}
                className='cursor-pointer'
                onClick={(event) => rowDisclosureClick(event, toggle)}
              >
                <TableCell className='max-w-105 whitespace-normal'>
                  <div className='flex flex-col gap-0.5'>
                    <span
                      className={cn(
                        'flex items-center gap-2 text-sm',
                        unread ? 'font-medium' : 'text-muted-foreground'
                      )}
                    >
                      {unread ? (
                        <span
                          aria-label={t('siteAnnouncements.row.unread')}
                          className='bg-primary size-1.5 shrink-0 rounded-full'
                        />
                      ) : null}
                      <RowDisclosureTrigger
                        expanded={expanded}
                        controls={contentId}
                        className='h-auto min-w-0 justify-start p-0 text-left text-sm font-medium break-words whitespace-normal'
                        onClick={toggle}
                      >
                        {title}
                      </RowDisclosureTrigger>
                      {lifecycle ? (
                        <Badge variant='outline'>
                          {t(
                            lifecycle === 'dismissed'
                              ? 'siteAnnouncements.row.dismissed'
                              : 'siteAnnouncements.row.expired'
                          )}
                        </Badge>
                      ) : null}
                    </span>
                    {item.content && !expanded ? (
                      <span
                        className='text-muted-foreground line-clamp-2 text-xs'
                        title={item.content}
                      >
                        {item.content}
                      </span>
                    ) : null}
                    <span className='text-muted-foreground mt-1 text-xs md:hidden'>
                      {siteById.get(item.siteId)?.name ?? item.platform} ·{' '}
                      {formatDateTime(item.firstSeenAt, locale)}
                    </span>
                    {(item.revisionCount ?? 0) > 0 ? (
                      <span className='text-muted-foreground mt-1 text-xs'>
                        {t('siteAnnouncements.timeline.updates', {
                          count: item.revisionCount,
                        })}
                      </span>
                    ) : null}
                  </div>
                </TableCell>
                <TableCell className='hidden md:table-cell'>
                  <div className='flex flex-col text-sm'>
                    <span>
                      {siteById.get(item.siteId)?.name ??
                        t('siteAnnouncements.row.siteUnknown', {
                          id: item.siteId,
                        })}
                    </span>
                    <span className='text-muted-foreground text-xs'>
                      {item.platform}
                    </span>
                  </div>
                </TableCell>
                <TableCell className='hidden md:table-cell'>
                  <LevelBadge level={item.level} />
                </TableCell>
                <TableCell className='text-muted-foreground hidden text-xs whitespace-nowrap md:table-cell'>
                  <SeenRange item={item} locale={locale} />
                </TableCell>
                <TableCell className='w-20 text-right md:w-auto'>
                  <div className='flex flex-col items-end justify-end gap-1 md:flex-row md:items-center'>
                    {sourceURL ? (
                      <a
                        href={sourceURL}
                        target='_blank'
                        rel='noopener noreferrer'
                        aria-label={t('siteAnnouncements.row.openUpstream')}
                        title={t('siteAnnouncements.row.openUpstream')}
                        className={buttonVariants({
                          variant: 'ghost',
                          size: 'icon-sm',
                        })}
                        onClick={() => {
                          if (unread) markRead(item.id)
                        }}
                      >
                        <ExternalLink aria-hidden='true' />
                      </a>
                    ) : null}
                    {unread ? (
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        disabled={markingRead}
                        onClick={() => markRead(item.id)}
                      >
                        {t('siteAnnouncements.row.markRead')}
                      </Button>
                    ) : null}
                  </div>
                </TableCell>
              </TableRow>
              {expanded ? (
                <TableRow>
                  <TableCell
                    colSpan={5}
                    className='max-w-0 py-4 whitespace-normal'
                  >
                    <RowDisclosureContent id={contentId} label={title}>
                      <AnnouncementDetails item={item} />
                    </RowDisclosureContent>
                  </TableCell>
                </TableRow>
              ) : null}
            </Fragment>
          )
        })}
      </TableBody>
    </Table>
  )
}
