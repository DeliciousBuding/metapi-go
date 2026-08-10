// metapi-go/routes — dashboard index (stub).
// Phase 2 replaces this with the real Dashboard (4-section overview).

import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

function DashboardStub() {
  const { t } = useTranslation()
  return (
    <div className='flex h-full items-center justify-center p-8'>
      <p className='text-muted-foreground text-lg'>
        {t('common.dashboardTbd')}
      </p>
    </div>
  )
}

export const Route = createFileRoute('/_authenticated/')({
  component: DashboardStub,
})
