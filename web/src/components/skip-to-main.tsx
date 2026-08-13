// metapi-go/components — skip-to-main accessibility link.
// Renders a visually-hidden link that becomes visible on focus, letting keyboard
// users jump straight to the main content region.

import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

export function SkipToMain() {
  const { t } = useTranslation()

  return (
    <a
      href='#content'
      className={cn(
        'bg-background text-foreground',
        'focus-visible:ring-ring focus-visible:ring-2 focus-visible:ring-offset-2',
        'pointer-events-none absolute left-4 top-4 z-[100]',
        '-translate-y-20 rounded-md border px-4 py-2 text-sm font-medium shadow-md',
        'transition-transform duration-150',
        'focus-visible:pointer-events-auto focus-visible:translate-y-0'
      )}
    >
      {t('common.skipToMain')}
    </a>
  )
}
