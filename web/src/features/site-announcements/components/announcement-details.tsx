import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { MarkdownContent } from '@/components/common/markdown-content'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Spinner } from '@/components/ui/spinner'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import { formatDateTime } from '@/lib/format'

import { siteAnnouncementsKeys, type SiteAnnouncement } from '../types'

export function AnnouncementDetails({ item }: { item: SiteAnnouncement }) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const history = useQuery({
    queryKey: [...siteAnnouncementsKeys.all, 'history', item.id],
    queryFn: () => api.getSiteAnnouncementHistory(item.id),
  })
  const current = history.data?.[0] ?? item
  const previous = history.data?.slice(1) ?? []
  return (
    <div className='min-w-0 space-y-6'>
      <div className='min-w-0'>
        <div className='text-muted-foreground mb-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs'>
          <span>{t('siteAnnouncements.timeline.current')}</span>
          <time dateTime={current.firstSeenAt}>
            {formatDateTime(current.firstSeenAt, locale)}
          </time>
        </div>
        <MarkdownContent>{current.content}</MarkdownContent>
        <p className='text-muted-foreground mt-3 text-xs'>
          {t('siteAnnouncements.timeline.lastSeen', {
            time: formatDateTime(current.lastSeenAt, locale),
          })}
        </p>
      </div>
      <QueryErrorBanner
        error={history.error}
        messageKey='siteAnnouncements.error.load'
        onRetry={() => void history.refetch()}
        isRetrying={history.isRefetching}
      />
      {history.isLoading ? (
        <div
          role='status'
          className='text-muted-foreground flex items-center gap-2 text-sm'
        >
          <Spinner />
          {t('common.loading')}
        </div>
      ) : null}
      {previous.length > 0 ? (
        <section
          aria-label={t('siteAnnouncements.timeline.title')}
          className='min-w-0 border-t pt-5'
        >
          <h3 className='mb-4 text-sm font-medium'>
            {t('siteAnnouncements.timeline.title')}
          </h3>
          <ol className='ml-1 min-w-0 border-l pl-5'>
            {previous.map((version) => (
              <li
                key={version.revisionId}
                className='relative min-w-0 pb-6 last:pb-0'
              >
                <span
                  aria-hidden='true'
                  className='bg-background border-muted-foreground absolute top-1 -left-6.5 size-3 rounded-full border-2'
                />
                <time
                  dateTime={version.firstSeenAt}
                  className='text-muted-foreground mb-3 block text-xs'
                >
                  {formatDateTime(version.firstSeenAt, locale)}
                </time>
                {version.title !== current.title ? (
                  <h4 className='mb-2 text-sm font-medium break-words'>
                    {version.title}
                  </h4>
                ) : null}
                <MarkdownContent>{version.content}</MarkdownContent>
              </li>
            ))}
          </ol>
        </section>
      ) : null}
    </div>
  )
}
