// metapi-go/features/dashboard/components — announcement banner.
//
// Ported from legacy web/components/AnnouncementBanner.tsx. Renders null
// while loading or when there are no active announcements (non-critical,
// silent failure on error — the banner never blocks the dashboard).
// Dismissal persists server-side via POST /api/announcements/{id}/dismiss;
// a failed dismiss keeps the banner visible and surfaces localized feedback
// instead of pretending the announcement was dismissed.
//
// Data flows through TanStack Query (like every other dashboard widget) so
// section switches reuse the cached banner instead of refetching on every
// mount; dismissal patches the cached list in place.

import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Info, Megaphone, X } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Notice, type NoticeTone } from '@/components/ui/notice'
import { api, type Announcement } from '@/lib/api'
import {
  getSafeProductAnnouncementUrl,
  productAnnouncementKeys,
} from '@/lib/product-announcements'
import { toast } from '@/lib/toast'

const SEVERITY_TONE: Record<
  Announcement['severity'],
  { tone: NoticeTone; icon: typeof Info }
> = {
  critical: { tone: 'destructive', icon: Megaphone },
  warning: { tone: 'warning', icon: Megaphone },
  info: { tone: 'info', icon: Info },
}

export function AnnouncementBanner() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dismissing, setDismissing] = useState<number | null>(null)

  const { data: items = [], isLoading } = useQuery({
    queryKey: productAnnouncementKeys.active(),
    queryFn: async () => {
      const response = await api.getActiveAnnouncements()
      return (response.items ?? []).filter((item) => !item.dismissed)
    },
    // Announcements change rarely (admin-managed banners); keep the cached
    // list across dashboard section switches for a minute before revalidating.
    staleTime: 60 * 1000,
  })

  if (isLoading || items.length === 0) return null

  const dismiss = async (id: number) => {
    setDismissing(id)
    try {
      await api.dismissAnnouncement(id)
      queryClient.setQueryData<Announcement[]>(
        productAnnouncementKeys.active(),
        (current) => (current ?? []).filter((item) => item.id !== id)
      )
    } catch {
      toast.error(t('dashboard.announcement.dismissFailed'))
      // Dismissal did not persist — keep the banner visible.
    } finally {
      setDismissing(null)
    }
  }

  return (
    <div className='flex flex-col gap-2'>
      {items.map((item) => {
        const tone = SEVERITY_TONE[item.severity] ?? SEVERITY_TONE.info
        const Icon = tone.icon
        const safeLink = getSafeProductAnnouncementUrl(item.link)
        return (
          <Notice
            key={item.id}
            tone={tone.tone}
            role='alert'
            className='gap-3 px-4 py-3'
          >
            <Icon className='mt-0.5 size-4 shrink-0' />
            <div className='min-w-0 flex-1'>
              <p className='font-medium'>{item.title}</p>
              {item.message ? (
                <p className='text-muted-foreground mt-0.5 text-xs'>
                  {item.message}
                </p>
              ) : null}
            </div>
            {safeLink ? (
              <a
                href={safeLink}
                target='_blank'
                rel='noopener noreferrer'
                className='text-xs underline underline-offset-2'
              >
                {t('dashboard.announcement.learnMore')}
              </a>
            ) : null}
            <button
              type='button'
              onClick={() => void dismiss(item.id)}
              disabled={dismissing === item.id}
              aria-label={t('dashboard.announcement.dismiss')}
              className='text-muted-foreground hover:text-foreground -mr-1 shrink-0 rounded p-1 disabled:opacity-50'
            >
              <X className='size-3.5' />
            </button>
          </Notice>
        )
      })}
    </div>
  )
}
