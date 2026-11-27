// metapi-go/features/site-announcements/components — upstream site
// announcements page (`/site-announcements`).
//
// Independent product surface for notices scraped from upstream sites
// (NewAPI-style /api/notice and friends). The operator risk banners served by
// /api/announcements live in Settings > Content and never intersect here.
//
// Titles and previews are plain text; expanded bodies use the shared Markdown renderer.

import { useNavigate, useSearch } from '@tanstack/react-router'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { PageHeader } from '@/components/common/page-header'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { useSites } from '@/features/sites'

import { useSiteAnnouncements } from '../api'
import {
  buildSiteAnnouncementsParams,
  SITE_ANNOUNCEMENTS_PAGE_SIZE,
  type SiteAnnouncementFilters,
} from '../types'
import {
  buildSiteAnnouncementsHref,
  parseSiteAnnouncementsSearch,
} from '../url-state'
import { AnnouncementFilters } from './announcement-filters'
import { SyncButton, useAnnouncementSync } from './announcement-sync'
import { AnnouncementTable } from './announcement-table'
import { useAnnouncementActions } from './use-announcement-actions'

export function SiteAnnouncementsPage() {
  const { t } = useTranslation()
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

  const sync = useAnnouncementSync(filters.siteId)
  const { markReadMutation, markAllMutation, clearMutation } =
    useAnnouncementActions()
  const unreadCount = items.filter((item) => !item.readAt).length

  // --- Render ------------------------------------------------------------------

  return (
    <div className='page-padding flex flex-col gap-4'>
      <PageHeader
        title={t('siteAnnouncements.page.title')}
        description={t('siteAnnouncements.page.description')}
        actions={
          <>
            {sync.status !== null ? (
              <span
                className='text-muted-foreground inline-flex items-center gap-2 text-xs'
                role='status'
              >
                <Spinner />
                {sync.status}
              </span>
            ) : null}
            <SyncButton sync={sync} />
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
          </>
        }
      />

      <AnnouncementFilters
        filters={filters}
        sites={sitesQuery.data}
        updateFilters={updateFilters}
      />
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
          {!isFiltered ? <SyncButton sync={sync} className='mt-2' /> : null}
        </div>
      ) : null}

      <AnnouncementTable
        items={listQuery.isLoading ? [] : items}
        sites={sitesQuery.data}
        markRead={markReadMutation.mutate}
        markingRead={markReadMutation.isPending}
      />
      {!listQuery.isLoading && (items.length > 0 || page > 0) ? (
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
