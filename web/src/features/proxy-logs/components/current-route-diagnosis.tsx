import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { buttonVariants } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'

export function CurrentRouteDiagnosis({
  model,
  endpoint,
  keyId,
}: {
  model: string | null
  endpoint: string | null
  keyId: number | null
}) {
  const { t } = useTranslation()
  if (!model || !endpoint) return null
  return (
    <>
      <Separator />
      <section className='flex flex-wrap items-center justify-between gap-2'>
        <div className='min-w-0'>
          <h3 className='text-sm font-medium'>
            {t('proxyLogs.detail.currentRoute.title')}
          </h3>
          <p className='text-muted-foreground text-xs'>
            {keyId
              ? t('proxyLogs.detail.currentRoute.keyHint')
              : t('proxyLogs.detail.currentRoute.adminHint')}
          </p>
        </div>
        <Link
          className={buttonVariants({ size: 'sm', variant: 'outline' })}
          to='/token-routes'
          search={{
            workspaceModel: model,
            workspaceEndpoint: endpoint,
            ...(keyId ? { workspaceKeyId: keyId } : {}),
          }}
        >
          {t('proxyLogs.detail.currentRoute.open')}
        </Link>
      </section>
    </>
  )
}
