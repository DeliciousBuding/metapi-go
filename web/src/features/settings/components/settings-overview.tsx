// metapi-go/features/settings — settings overview landing.
//
// Rendered at bare `/settings`: a card grid of the 5 subareas so the full
// configuration scope is visible before drilling in. Cards derive their
// metadata (title / icon / description / section list) from the shared
// subarea manifest, so the overview stays in sync with the main sidebar and
// the per-subarea registries without a second copy.

import { Link, type LinkProps } from '@tanstack/react-router'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'

import { getSettingsSubareas } from '../config/settings-config'

export function SettingsOverview() {
  const { t } = useTranslation()
  const subareas = getSettingsSubareas()

  return (
    <div className='flex flex-col gap-6 p-6'>
      <header className='flex flex-col gap-1'>
        <h1 className='text-2xl font-normal tracking-tight'>
          {t('settings.overview.title')}
        </h1>
        <p className='text-muted-foreground text-sm'>
          {t('settings.overview.description')}
        </p>
      </header>
      <div className='grid gap-4 sm:grid-cols-2 xl:grid-cols-3'>
        {subareas.map((subarea) => {
          const Icon = subarea.icon
          const sections = subarea.getSectionNavItems()
          return (
            <Card key={subarea.id} size='sm' className='h-full'>
              <CardContent className='flex h-full flex-col gap-3'>
                <Link
                  to={
                    `${subarea.basePath}/${subarea.defaultSection}` as
                      | LinkProps['to']
                      | (string & {})
                  }
                  className='group/subarea flex items-center gap-2 font-medium'
                >
                  {Icon ? (
                    <Icon className='text-primary size-4 shrink-0' />
                  ) : null}
                  <span className='min-w-0 truncate'>{t(subarea.title)}</span>
                  <ChevronRight className='text-muted-foreground ml-auto size-4 shrink-0 transition-transform group-hover/subarea:translate-x-0.5' />
                </Link>
                {subarea.description ? (
                  <p className='text-muted-foreground text-sm'>
                    {t(subarea.description)}
                  </p>
                ) : null}
                <ul className='mt-auto flex flex-col gap-1 border-t pt-3'>
                  {sections.map((section) => (
                    <li key={String(section.url)}>
                      <Link
                        to={section.url}
                        className='text-muted-foreground hover:text-foreground flex items-center gap-2 rounded-md px-1 py-1 text-sm transition-colors'
                      >
                        {t(section.title)}
                      </Link>
                    </li>
                  ))}
                </ul>
              </CardContent>
            </Card>
          )
        })}
      </div>
    </div>
  )
}
