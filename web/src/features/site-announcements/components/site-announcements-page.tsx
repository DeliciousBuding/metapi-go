// metapi-go/features/site-announcements/components — upstream site
// announcements page (`/site-announcements`).
//
// Independent product surface for notices scraped from upstream sites
// (NewAPI-style /api/notice and friends). The operator risk banners served by
// /api/announcements live in Settings > Content and never intersect here.
//
// SECURITY: title / content / source fields are UNTRUSTED upstream data.
// The body is rendered as plain text only (never dangerouslySetInnerHTML,
// never markdown), and `sourceUrl` is deliberately NOT rendered as a
// clickable external link unless it resolves safely against the trusted Site URL.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import {
  ChevronLeft,
  ChevronRight,
  ExternalLink,
  RefreshCw,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useSites } from '@/features/sites/api'
import type { Site } from '@/features/sites/types'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'

import { resolveAnnouncementSourceURL } from '../announcement-source-url'
import { useSiteAnnouncements, useSiteAnnouncementSyncTask } from '../api'
import {
  buildSiteAnnouncementsParams,
  SITE_ANNOUNCEMENTS_PAGE_SIZE,
  siteAnnouncementsKeys,
  TERMINAL_SYNC_TASK_STATUSES,
  type SiteAnnouncement,
  type SiteAnnouncementFilters,
  type SiteAnnouncementSyncTask,
} from '../types'
import {
  buildSiteAnnouncementsHref,
  parseSiteAnnouncementsSearch,
} from '../url-state'

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

/** i18n key for the inline sync progress chip fallback label. */
function syncStatusKey(syncTask: SiteAnnouncementSyncTask | undefined): string {
  if (syncTask && syncTask.status === 'running') {
    return 'siteAnnouncements.sync.running'
  }
  return 'siteAnnouncements.sync.pending'
}

export function SiteAnnouncementsPage() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  const search = useSearch({ from: '/_authenticated/site-announcements' })
  const { filters, page } = useMemo(
    () => parseSiteAnnouncementsSearch(search),
    [search]
  )
  const [clearConfirmOpen, setClearConfirmOpen] = useState(false)

  const params = useMemo(
    () => buildSiteAnnouncementsParams(page, filters),
    [page, filters]
  )
  const listQuery = useSiteAnnouncements(params)
  const sitesQuery = useSites()

  const rows = listQuery.data ?? []
  const hasMore = rows.length > SITE_ANNOUNCEMENTS_PAGE_SIZE
  const items = hasMore ? rows.slice(0, SITE_ANNOUNCEMENTS_PAGE_SIZE) : rows

  const siteById = useMemo(() => {
    const map = new Map<number, Site>()
    for (const site of sitesQuery.data ?? []) {
      map.set(site.id, site)
    }
    return map
  }, [sitesQuery.data])

  const platformOptions = useMemo(() => {
    const set = new Set<string>()
    for (const site of sitesQuery.data ?? []) {
      if (site.platform) set.add(site.platform)
    }
    return [...set].sort((left, right) => left.localeCompare(right))
  }, [sitesQuery.data])

  const isFiltered =
    filters.siteId !== null ||
    filters.platform !== '' ||
    filters.read !== 'all' ||
    filters.status !== 'all' ||
    // A deep-linked stale page cursor behaves like a filter: the "no matches"
    // variant is less misleading than "nothing synced yet" + the Sync CTA.
    page > 0

  function updateFilters(patch: Partial<SiteAnnouncementFilters>) {
    const next = { ...filters, ...patch }
    void navigate({ href: buildSiteAnnouncementsHref(next, 0), replace: true })
  }

  // --- Background sync (POST /sync → poll api.getTask) ---------------------

  const [syncTaskId, setSyncTaskId] = useState<string | null>(null)
  const syncTaskQuery = useSiteAnnouncementSyncTask(syncTaskId)
  const syncTask = syncTaskQuery.data
  // Active from the moment the task id is known until the polled task reaches
  // a terminal status — covers the window between the POST response and the
  // first poll result where `syncTask` is still undefined.
  const syncIsActive =
    syncTaskId !== null &&
    !syncTaskQuery.isError &&
    (syncTask === undefined ||
      !TERMINAL_SYNC_TASK_STATUSES.has(syncTask.status))

  const handledTerminalRef = useRef<string | null>(null)
  useEffect(() => {
    if (!syncTask || !TERMINAL_SYNC_TASK_STATUSES.has(syncTask.status)) return
    const terminalKey = `${syncTask.id}:${syncTask.status}`
    if (handledTerminalRef.current === terminalKey) return
    handledTerminalRef.current = terminalKey
    void queryClient.invalidateQueries({ queryKey: siteAnnouncementsKeys.all })
    if (syncTask.status === 'succeeded') {
      const result = syncTask.result
      if (result && result.failed > 0) {
        toast.warning(
          t('siteAnnouncements.sync.successWithFailures', {
            inserted: result.inserted,
            updated: result.updated,
            failed: result.failed,
          })
        )
      } else {
        toast.success(
          t('siteAnnouncements.sync.success', {
            inserted: result?.inserted ?? 0,
            updated: result?.updated ?? 0,
          })
        )
      }
    } else {
      toast.error(syncTask.error || t('siteAnnouncements.sync.failed'))
    }
  }, [syncTask, queryClient, t])

  const syncMutation = useMutation({
    mutationFn: async () =>
      api.syncSiteAnnouncements(
        filters.siteId !== null ? { siteId: filters.siteId } : undefined
      ),
    onSuccess: (response) => {
      handledTerminalRef.current = null
      setSyncTaskId(response.taskId)
    },
    onError: () => toast.error(t('siteAnnouncements.sync.startFailed')),
  })

  // --- Write mutations -------------------------------------------------------

  const markReadMutation = useMutation({
    mutationFn: async (id: number) => api.markSiteAnnouncementRead(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: siteAnnouncementsKeys.all,
      })
      void queryClient.invalidateQueries({ queryKey: ['dashboard-attention'] })
    },
    onError: () => toast.error(t('siteAnnouncements.toast.markReadFailed')),
  })

  const markAllMutation = useMutation({
    mutationFn: async () => api.markAllSiteAnnouncementsRead(),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: siteAnnouncementsKeys.all,
      })
      void queryClient.invalidateQueries({ queryKey: ['dashboard-attention'] })
      toast.success(t('siteAnnouncements.toast.allMarkedRead'))
    },
    onError: () => toast.error(t('siteAnnouncements.toast.markAllFailed')),
  })

  const clearMutation = useMutation({
    mutationFn: async () => api.clearSiteAnnouncements(),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: siteAnnouncementsKeys.all,
      })
      void queryClient.invalidateQueries({ queryKey: ['dashboard-attention'] })
      toast.success(t('siteAnnouncements.toast.cleared'))
    },
    onError: () => toast.error(t('siteAnnouncements.toast.clearFailed')),
  })

  const unreadCount = items.filter((item) => !item.readAt).length

  // --- Render ------------------------------------------------------------------

  return (
    <div className='flex flex-col gap-4 p-4 md:p-6'>
      <header className='flex flex-wrap items-start justify-between gap-3'>
        <div>
          <h1 className='text-lg font-normal tracking-tight'>
            {t('siteAnnouncements.page.title')}
          </h1>
          <p className='text-muted-foreground text-sm'>
            {t('siteAnnouncements.page.description')}
          </p>
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          {syncIsActive ? (
            <span
              className='text-muted-foreground inline-flex items-center gap-2 text-xs'
              role='status'
            >
              <Spinner />
              {syncTask?.message || t(syncStatusKey(syncTask))}
            </span>
          ) : null}
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={syncMutation.isPending || syncIsActive}
            onClick={() => syncMutation.mutate()}
          >
            {syncMutation.isPending || syncIsActive ? (
              <Spinner />
            ) : (
              <RefreshCw className='size-3.5' />
            )}
            {t('siteAnnouncements.actions.sync')}
          </Button>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={markAllMutation.isPending || unreadCount === 0}
            onClick={() => markAllMutation.mutate()}
          >
            {t('siteAnnouncements.actions.markAllRead')}
          </Button>
          <Button
            type='button'
            variant='destructive'
            size='sm'
            disabled={clearMutation.isPending}
            onClick={() => setClearConfirmOpen(true)}
          >
            {t('siteAnnouncements.actions.clear')}
          </Button>
        </div>
      </header>

      <div className='flex flex-wrap items-center gap-3'>
        <Select
          value={filters.siteId === null ? 'all' : String(filters.siteId)}
          onValueChange={(value) =>
            updateFilters({
              siteId: !value || value === 'all' ? null : Number(value),
            })
          }
        >
          <SelectTrigger
            aria-label={t('siteAnnouncements.filters.site')}
            className='w-44'
          >
            {/* base-ui only resolves the item label while the popup is
                mounted; map the value explicitly so the closed trigger
                never falls back to the raw "all" sentinel. */}
            <SelectValue>
              {(value: string) =>
                value === 'all'
                  ? t('siteAnnouncements.filters.allSites')
                  : ((sitesQuery.data ?? []).find(
                      (site) => String(site.id) === value
                    )?.name ?? value)
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>
              {t('siteAnnouncements.filters.allSites')}
            </SelectItem>
            {(sitesQuery.data ?? []).map((site) => (
              <SelectItem key={site.id} value={String(site.id)}>
                {site.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={filters.platform || 'all'}
          onValueChange={(value) =>
            updateFilters({ platform: !value || value === 'all' ? '' : value })
          }
        >
          <SelectTrigger
            aria-label={t('siteAnnouncements.filters.platform')}
            className='w-40'
          >
            <SelectValue>
              {(value: string) =>
                value === 'all'
                  ? t('siteAnnouncements.filters.allPlatforms')
                  : value
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>
              {t('siteAnnouncements.filters.allPlatforms')}
            </SelectItem>
            {platformOptions.map((platform) => (
              <SelectItem key={platform} value={platform}>
                {platform}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={filters.read}
          onValueChange={(value) =>
            updateFilters({
              read: (value === 'true' || value === 'false'
                ? value
                : 'all') as SiteAnnouncementFilters['read'],
            })
          }
        >
          <SelectTrigger
            aria-label={t('siteAnnouncements.filters.read')}
            className='w-36'
          >
            <SelectValue>
              {(value: string) => {
                if (value === 'true') {
                  return t('siteAnnouncements.filters.readTrue')
                }
                if (value === 'false') {
                  return t('siteAnnouncements.filters.readFalse')
                }
                return t('siteAnnouncements.filters.readAll')
              }}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>
              {t('siteAnnouncements.filters.readAll')}
            </SelectItem>
            <SelectItem value='false'>
              {t('siteAnnouncements.filters.readFalse')}
            </SelectItem>
            <SelectItem value='true'>
              {t('siteAnnouncements.filters.readTrue')}
            </SelectItem>
          </SelectContent>
        </Select>

        <Select
          value={filters.status}
          onValueChange={(value) =>
            updateFilters({
              status: (value === 'active' ||
              value === 'expired' ||
              value === 'dismissed'
                ? value
                : 'all') as SiteAnnouncementFilters['status'],
            })
          }
        >
          <SelectTrigger
            aria-label={t('siteAnnouncements.filters.status')}
            className='w-36'
          >
            <SelectValue>
              {(value: string) => {
                if (value === 'active') {
                  return t('siteAnnouncements.filters.statusActive')
                }
                if (value === 'expired') {
                  return t('siteAnnouncements.filters.statusExpired')
                }
                if (value === 'dismissed') {
                  return t('siteAnnouncements.filters.statusDismissed')
                }
                return t('siteAnnouncements.filters.statusAll')
              }}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>
              {t('siteAnnouncements.filters.statusAll')}
            </SelectItem>
            <SelectItem value='active'>
              {t('siteAnnouncements.filters.statusActive')}
            </SelectItem>
            <SelectItem value='expired'>
              {t('siteAnnouncements.filters.statusExpired')}
            </SelectItem>
            <SelectItem value='dismissed'>
              {t('siteAnnouncements.filters.statusDismissed')}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>

      <QueryErrorBanner
        error={listQuery.error}
        messageKey='siteAnnouncements.error.load'
        onRetry={() => void listQuery.refetch()}
        isRetrying={listQuery.isRefetching}
      />

      {listQuery.isLoading ? (
        <div className='flex flex-col gap-2' aria-busy='true'>
          {[0, 1, 2, 3].map((rowIndex) => (
            <Skeleton key={rowIndex} className='h-14 w-full' />
          ))}
        </div>
      ) : null}

      {!listQuery.isLoading && !listQuery.error && items.length === 0 ? (
        <div className='text-muted-foreground flex flex-col items-center gap-2 py-12 text-center'>
          <p className='text-foreground text-sm font-medium'>
            {isFiltered
              ? t('siteAnnouncements.empty.filteredTitle')
              : t('siteAnnouncements.empty.title')}
          </p>
          <p className='max-w-md text-sm'>
            {isFiltered
              ? t('siteAnnouncements.empty.filteredDescription')
              : t('siteAnnouncements.empty.description')}
          </p>
          {!isFiltered ? (
            <Button
              variant='outline'
              size='sm'
              className='mt-2'
              disabled={syncMutation.isPending || syncIsActive}
              onClick={() => syncMutation.mutate()}
            >
              {syncMutation.isPending || syncIsActive ? (
                <Spinner />
              ) : (
                <RefreshCw className='size-3.5' />
              )}
              {t('siteAnnouncements.actions.sync')}
            </Button>
          ) : null}
        </div>
      ) : null}

      {!listQuery.isLoading && items.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                {t('siteAnnouncements.columns.announcement')}
              </TableHead>
              <TableHead>{t('siteAnnouncements.columns.site')}</TableHead>
              <TableHead>{t('siteAnnouncements.columns.level')}</TableHead>
              <TableHead>{t('siteAnnouncements.columns.seen')}</TableHead>
              <TableHead className='text-right'>
                {t('siteAnnouncements.columns.actions')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((item) => {
              const unread = !item.readAt
              const lifecycle = rowLifecycle(item)
              const sourceURL = resolveAnnouncementSourceURL(
                item,
                siteById.get(item.siteId)
              )
              return (
                <TableRow key={item.id} data-unread={unread || undefined}>
                  <TableCell className='max-w-[420px]'>
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
                        {/* Untrusted upstream content: plain text only. */}
                        <span className='truncate' title={item.title}>
                          {item.title}
                        </span>
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
                      {item.content ? (
                        <span
                          className='text-muted-foreground line-clamp-2 text-xs'
                          title={item.content}
                        >
                          {item.content}
                        </span>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell>
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
                  <TableCell>
                    <LevelBadge level={item.level} />
                  </TableCell>
                  <TableCell className='text-muted-foreground text-xs whitespace-nowrap'>
                    <SeenRange item={item} locale={locale} />
                  </TableCell>
                  <TableCell className='text-right'>
                    <div className='flex items-center justify-end gap-1'>
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
                            if (unread) markReadMutation.mutate(item.id)
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
                          disabled={markReadMutation.isPending}
                          onClick={() => markReadMutation.mutate(item.id)}
                        >
                          {t('siteAnnouncements.row.markRead')}
                        </Button>
                      ) : null}
                    </div>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      ) : null}

      {!listQuery.isLoading && items.length > 0 ? (
        <div className='flex items-center justify-between'>
          <span className='text-muted-foreground text-xs'>
            {t('siteAnnouncements.pagination.summary', {
              count: items.length,
            })}
          </span>
          <div className='flex items-center gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={page === 0}
              onClick={() =>
                void navigate({
                  href: buildSiteAnnouncementsHref(
                    filters,
                    Math.max(0, page - 1)
                  ),
                  replace: true,
                })
              }
            >
              <ChevronLeft className='size-3.5' />
              {t('siteAnnouncements.pagination.previous')}
            </Button>
            <span className='text-muted-foreground text-xs'>
              {t('siteAnnouncements.pagination.page', { page: page + 1 })}
            </span>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={!hasMore}
              onClick={() =>
                void navigate({
                  href: buildSiteAnnouncementsHref(filters, page + 1),
                  replace: true,
                })
              }
            >
              {t('siteAnnouncements.pagination.next')}
              <ChevronRight className='size-3.5' />
            </Button>
          </div>
        </div>
      ) : null}

      <ConfirmDialog
        open={clearConfirmOpen}
        title={t('siteAnnouncements.clear.title')}
        description={t('siteAnnouncements.clear.description')}
        confirmLabel={t('siteAnnouncements.clear.confirm')}
        cancelLabel={t('common.cancel')}
        destructive
        onConfirm={() => {
          setClearConfirmOpen(false)
          clearMutation.mutate()
        }}
        onCancel={() => setClearConfirmOpen(false)}
      />
    </div>
  )
}
