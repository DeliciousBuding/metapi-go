// metapi-go/features/observability/sections/proxy-logs — pointer to the full
// proxy request log workspace. The existing /proxy-logs page owns its own
// URL state and single h1, so the Observability workspace links out instead
// of embedding it and breaking the page-title / URL-state contracts.

import { Link } from '@tanstack/react-router'
import { ScrollText } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { buttonVariants } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

export function ProxyLogsSection() {
  const { t } = useTranslation()

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center gap-2 text-sm font-medium'>
          <ScrollText className='size-4' />
          {t('observability.proxyLogs.title')}
        </CardTitle>
        <CardDescription className='text-xs'>
          {t('observability.proxyLogs.description')}
        </CardDescription>
      </CardHeader>
      <CardContent className='flex flex-col items-start gap-3'>
        <p className='text-muted-foreground text-sm'>
          {t('observability.proxyLogs.hint')}
        </p>
        <Link
          to='/proxy-logs'
          className={buttonVariants({ variant: 'outline', size: 'sm' })}
        >
          {t('observability.proxyLogs.open')}
        </Link>
      </CardContent>
    </Card>
  )
}
