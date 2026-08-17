// metapi-go/features/settings — in-page section sidebar.
//
// Renders the current subarea's sections as the secondary navigation inside
// the Settings workspace: once the user has drilled into a subarea (e.g.
// /settings/general), this nav lists that subarea's sections and highlights
// the active one. It complements the main app sidebar (which lists the 5
// subareas themselves).
//
// Responsive contract (audit P2 #6 closeout): at `lg` and above this is the
// classic sticky vertical sidebar (w-60); below `lg` the same DOM degrades
// into a single-row horizontally scrollable chip strip so the section nav
// never buries the page content on a 375px viewport (a full-height vertical
// list pushed the first section card below the fold). Group labels collapse
// on mobile — chips are self-explanatory and labels would add scroll noise.
//
// Active-state is derived from the live URL so the nav stays correct across
// browser back/forward and direct deep links; the active link additionally
// exposes `aria-current="page"`.

import { Link, useLocation } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { SettingsSectionNavItem } from '../types'

type SettingsSidebarProps = {
  items: SettingsSectionNavItem[]
  /** Optional group label rendered above the section list (desktop only). */
  title?: string
}

export function SettingsSidebar({ items, title }: SettingsSidebarProps) {
  const { t } = useTranslation()
  const href = useLocation({ select: (location) => location.href })
  const activeHref = normalizePath(href)

  const groups = useMemo(() => {
    const map = new Map<string | undefined, SettingsSectionNavItem[]>()
    for (const item of items) {
      const key = item.group ?? undefined
      const bucket = map.get(key) ?? []
      bucket.push(item)
      map.set(key, bucket)
    }
    return [...map.entries()]
  }, [items])

  return (
    <aside className='w-full shrink-0 lg:sticky lg:top-6 lg:w-60'>
      <nav className='flex flex-row gap-1 overflow-x-auto lg:flex-col lg:overflow-visible'>
        {title ? (
          <p className='text-muted-foreground/70 hidden px-3 pb-2 text-[11px] font-medium tracking-wider uppercase lg:block'>
            {title}
          </p>
        ) : null}
        {groups.map(([groupKey, groupItems]) => (
          <div
            key={groupKey ?? '__ungrouped__'}
            className='flex shrink-0 flex-row gap-1 lg:flex-col'
          >
            {groupKey ? (
              <p className='text-muted-foreground/75 hidden px-3 pt-2 pb-1 text-[11px] font-medium tracking-wider uppercase lg:block'>
                {t(groupKey)}
              </p>
            ) : null}
            {groupItems.map((item) => {
              const itemHref = normalizePath(String(item.url))
              const isActive = activeHref === itemHref
              return (
                <Link
                  key={String(item.url)}
                  to={item.url}
                  aria-current={isActive ? 'page' : undefined}
                  className={cn(
                    'flex shrink-0 items-center gap-2 rounded-full px-3 py-1.5 text-sm whitespace-nowrap transition-colors lg:w-full lg:rounded-md lg:py-2 lg:whitespace-normal',
                    isActive
                      ? 'bg-accent font-medium text-accent-foreground'
                      : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                  )}
                >
                  <span className='min-w-0 truncate lg:flex-1'>
                    {t(item.title)}
                  </span>
                  {item.readonly ? (
                    <span className='bg-muted text-muted-foreground shrink-0 rounded-sm px-1 py-0.5 text-[10px]'>
                      {t('settings.common.readonly')}
                    </span>
                  ) : null}
                </Link>
              )
            })}
          </div>
        ))}
      </nav>
    </aside>
  )
}

/** Strip the query string and trailing slashes for stable active-state matching. */
function normalizePath(href: string): string {
  const withoutQuery = href.split('?')[0]
  return withoutQuery.length > 1
    ? withoutQuery.replace(/\/+$/, '')
    : withoutQuery
}
