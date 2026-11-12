import { Link, useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { AdminAuditLogTable } from './admin-audit-log-table'
import { GatewayRequestsPage } from './gateway-requests-page'

function AuditNavigation({ category }: { category: 'access' | 'operations' }) {
  const { t } = useTranslation()
  return (
    <nav aria-label={t('auditLogs.categories')} className='flex gap-1'>
      {(['operations', 'access'] as const).map((target) => (
        <Link
          key={target}
          to='/audit-logs'
          search={{ category: target }}
          aria-current={category === target ? 'page' : undefined}
          className={cn(
            buttonVariants({ size: 'sm', variant: 'ghost' }),
            category === target && 'bg-muted'
          )}
        >
          {t(`auditLogs.${target}`)}
        </Link>
      ))}
    </nav>
  )
}

export function AuditLogsPage() {
  const location = useLocation()
  const { t } = useTranslation()
  const category =
    new URLSearchParams(location.searchStr).get('category') === 'access'
      ? 'access'
      : 'operations'
  const navigation = <AuditNavigation category={category} />
  if (category === 'access') {
    return <GatewayRequestsPage kind='access' navigation={navigation} />
  }
  return (
    <div className='page-padding flex min-w-0 flex-col gap-4'>
      <PageHeader
        title={t('auditLogs.title')}
        description={t('auditLogs.operationsDescription')}
      >
        {navigation}
      </PageHeader>
      <AdminAuditLogTable />
    </div>
  )
}
