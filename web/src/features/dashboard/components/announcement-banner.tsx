// metapi-go/features/dashboard/components — announcement banner.
//
// Ported from legacy web/components/AnnouncementBanner.tsx. Renders null
// while loading or when there are no active announcements (non-critical,
// silent failure on error — the banner never blocks the dashboard).
//
// Phase 2: fetches api.getActiveAnnouncements() but renders a stub empty
// state; phase 3 will wire the dismiss call (api.dismissAnnouncement) and
// markdown rendering via @/components/ui/markdown.

import { useEffect, useState } from 'react'
import { Info, Megaphone, X } from 'lucide-react'

import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

import type { AnnouncementItem } from '../types'

const SEVERITY_TONE: Record<
  AnnouncementItem['severity'],
  { wrapper: string; icon: typeof Info }
> = {
  critical: {
    wrapper:
      'border-destructive/40 bg-destructive/10 text-destructive',
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
  const [items, setItems] = useState<AnnouncementItem[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    api
      .getActiveAnnouncements()
      .then((response) => {
        if (cancelled) return
        // The api returns { items: Announcement[] }; narrow to the local
        // shape (TODO phase 3: align on a shared Announcement type).
        const next = ((response as { items?: AnnouncementItem[] }).items ??
          []) as AnnouncementItem[]
        setItems(next.filter((item) => !item.dismissed))
      })
      .catch(() => {
        // Silent degrade — the banner is never a blocker.
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (loading || items.length === 0) return null

  const dismiss = (id: number) => {
    // TODO phase 3: api.dismissAnnouncement(id) once the method is wired.
    setItems((prev) => prev.filter((item) => item.id !== id))
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
              tone.wrapper,
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
                learn more
              </a>
            ) : null}
            <button
              type='button'
              onClick={() => dismiss(item.id)}
              aria-label='Dismiss announcement'
              className='text-muted-foreground hover:text-foreground -mr-1 shrink-0 rounded p-1'
            >
              <X className='size-3.5' />
            </button>
          </div>
        )
      })}
    </div>
  )
}
