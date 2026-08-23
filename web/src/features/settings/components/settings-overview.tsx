// metapi-go/features/settings — settings overview landing.
//
// Rendered at bare `/settings`: a grid of 5 icon tiles (one per subarea,
// icon + title + description). The per-subarea section lists were removed
// (wave 8 lane C) — the sidebar's collapsible tree is now the single
// navigation surface, and the tiles only carry the "first location +
// description" duty. Tile metadata derives from the shared subarea
// manifest, so the overview stays in sync with the main sidebar and the
// per-subarea registries without a second copy.

import { Link, type LinkProps } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { getSettingsSubareas } from '../config/settings-config'

export function SettingsOverview() {
  const { t } = useTranslation()
  const subareas = getSettingsSubareas()

  return (
    <div className='flex flex-col gap-6 p-6'>
      <header className='flex flex-col gap-1'>
        <h1 className='text-lg font-bold tracking-tight sm:text-xl'>
          {t('settings.overview.title')}
        </h1>
        <p className='text-muted-foreground text-sm'>
          {t('settings.overview.description')}
        </p>
      </header>
      {/* 5 tiles in one row at xl (no 3+2 ragged rows); 2-col from sm;
          single column on mobile. Each tile is one whole-card link. */}
      <div className='grid gap-4 sm:grid-cols-2 xl:grid-cols-5'>
        {subareas.map((subarea) => {
          const Icon = subarea.icon
          return (
            <Link
              key={subarea.id}
              to={
                `${subarea.basePath}/${subarea.defaultSection}` as
                  | LinkProps['to']
                  | (string & {})
              }
              className='focus-visible:ring-ring/50 bg-card text-card-foreground ring-foreground/10 hover:bg-accent hover:text-accent-foreground flex h-full flex-col gap-2 rounded-xl p-4 text-sm ring-1 transition-colors focus-visible:ring-2 focus-visible:outline-none'
            >
              {Icon ? <Icon className='text-primary size-5 shrink-0' /> : null}
              <span className='font-medium'>{t(subarea.title)}</span>
              {subarea.description ? (
                <span className='text-muted-foreground text-sm'>
                  {t(subarea.description)}
                </span>
              ) : null}
            </Link>
          )
        })}
      </div>
    </div>
  )
}
