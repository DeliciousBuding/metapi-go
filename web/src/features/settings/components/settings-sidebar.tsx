// metapi-go/features/settings — in-page section sidebar.
//
// Renders the current subarea's sections as a vertical nav. This is the
// secondary navigation shown inside the Settings workspace: once the user has
// drilled into a subarea (e.g. /settings/general), this sidebar lists that
// subarea's sections and highlights the active one. It complements the main
// app sidebar (which lists the 5 subareas themselves).
//
// Active-state is derived from the live URL so the sidebar stays correct
// across browser back/forward and direct deep links.

import { Link, useLocation } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { SettingsSectionNavItem } from '../types'

type SettingsSidebarProps = {
  items: SettingsSectionNavItem[]
  /** Optional group label rendered above the section list. */
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
      <nav className='flex flex-col gap-1'>
        {title ? (
          <p className='text-muted-foreground/70 px-3 pb-2 text-[11px] font-medium tracking-wider uppercase'>
            {title}
          </p>
        ) : null}
        {groups.map(([groupKey, groupItems]) => (
          <div
            key={groupKey ?? '__ungrouped__'}
            className='flex flex-col gap-1'
          >
            {groupKey ? (
              <p className='text-muted-foreground/60 px-3 pt-2 pb-1 text-[11px] font-medium tracking-wider uppercase'>
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
                  className={cn(
                    'flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
                    isActive
                      ? 'bg-accent font-medium text-accent-foreground'
                      : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                  )}
                >
                  <span className='min-w-0 flex-1 truncate'>
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
