import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { proxyLogsSearchSchema } from '@/features/proxy-logs'
import { AuditLogsPage } from '@/features/proxy-logs/components/audit-logs-page'
import { stringSearchParam } from '@/lib/helpers/searchParams'

export const Route = createFileRoute('/_authenticated/audit-logs')({
  validateSearch: proxyLogsSearchSchema.extend({
    category: z
      .enum(['access', 'operations'])
      .catch('operations')
      .default('operations'),
    method: z
      .enum(['all', 'POST', 'PUT', 'PATCH', 'DELETE'])
      .catch('all')
      .default('all'),
    path: stringSearchParam,
  }),
  staticData: { title: 'auditLogs.title' },
  component: AuditLogsPage,
})
