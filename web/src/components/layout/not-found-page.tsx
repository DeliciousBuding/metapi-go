// metapi-go/components/layout — route-level 404 page.
//
// Mounted via the router's defaultNotFoundComponent so unknown routes get a
// recovery path instead of a white screen. Kept i18n-driven like the rest of
// the console.

import { Link } from '@tanstack/react-router'
import { Compass, House } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

export function NotFoundPage() {
  const { t } = useTranslation()
  return (
    <div className='flex min-h-screen flex-col items-center justify-center gap-4 p-6'>
      <div className='bg-muted/60 flex size-16 items-center justify-center rounded-2xl'>
        <Compass className='text-muted-foreground size-8' />
      </div>
      <div className='space-y-1 text-center'>
        <p className='text-2xl font-semibold'>{t('errors.notFoundTitle')}</p>
        <p className='text-muted-foreground text-sm'>
          {t('errors.notFoundDescription')}
        </p>
      </div>
      <Button render={<Link to='/' />}>
        <House className='mr-1.5 size-4' />
        {t('errors.backHome')}
      </Button>
    </div>
  )
}
