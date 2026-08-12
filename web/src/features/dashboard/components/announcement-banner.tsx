// metapi-go/features/dashboard/components — announcement banner.
//
// Ported from legacy web/components/AnnouncementBanner.tsx. Renders null
// while loading or when there are no active announcements (non-critical,
// silent failure on error — the banner never blocks the dashboard).
// Dismissal persists server-side via POST /api/announcements/{id}/dismiss;
// a failed dismiss keeps the banner visible (silent degrade) instead of
// pretending the announcement was dismissed.

import { Info, Megaphone, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { api, type Announcement } from '@/lib/api'
import { cn } from '@/lib/utils'

const SEVERITY_TONE: Record<
  Announcement['severity'],
  { wrapper: string; icon: typeof Info }
> = {
  critical: {
    wrapper: 'border-destructive/40 bg-destructive/10 text-destructive-soft-fg',
    icon: Megaphone,
  },
  warning: {
    wrapper: 'border-warning/40 bg-warning/10 text-warning-foreground',
    icon: Megaphone,
  },
  info: {
    wrapper: 'border-info/40 bg-info/10 text-info-foreground',
    icon: Info,
  },
}

export function AnnouncementBanner() {
  const { t } = useTranslation()
  const [items, setItems] = useState<Announcement[]>([])
  const [loading, setLoading] = useState(true)
  const [dismissing, setDismissing] = useState<number | null>(null)

  useEffect(() => {
    let cancelled = false
    async function load() {
      setLoading(true)
      try {
        const response = await api.getActiveAnnouncements()
        if (cancelled) return
        setItems((response.items ?? []).filter((item) => !item.dismissed))
      } catch {
        // Silent degrade — the banner is never a blocker.
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [])

  if (loading || items.length === 0) return null

  const dismiss = async (id: number) => {
    setDismissing(id)
    try {
      await api.dismissAnnouncement(id)
      setItems((prev) => prev.filter((item) => item.id !== id))
    } catch {
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
        return (
          <div
            key={item.id}
            role='alert'
            className={cn(
              'flex items-start gap-3 rounded-lg border px-4 py-3 text-sm',
              tone.wrapper
            )}
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
            {item.link ? (
              <a
                href={item.link}
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
          </div>
        )
      })}
    </div>
  )
}
