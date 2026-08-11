// metapi-go/features/settings/sections/system-info/components — admin audit
// logs section (B1). Read-only table of admin write operations with a method
// filter + path text search.

import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

type AuditLogItem = {
  id: number
  actor?: string
  method: string
  path: string
  status: number
  requestId?: string
  remoteIp?: string
  createdAt: string
}

type AuditLogsResponse = { items: AuditLogItem[]; total?: number; limit?: number }

const auditQueryKeys = {
  all: ['admin-audit-logs'] as const,
  list: (filter: string) => [...auditQueryKeys.all, 'list', filter] as const,
}

const METHOD_FILTERS = ['all', 'POST', 'PUT', 'PATCH', 'DELETE'] as const

export function AuditLogsSection() {
  const { t } = useTranslation()
  const [methodFilter, setMethodFilter] = useState<string>('all')
  const [pathSearch, setPathSearch] = useState('')
  const [submittedPath, setSubmittedPath] = useState('')

  const filterString = (() => {
    const params = new URLSearchParams()
    if (methodFilter !== 'all') {
      params.set('method', methodFilter)
    }
    if (submittedPath) {
      params.set('path', submittedPath)
    }
    return params.toString()
  })()

  const auditQuery = useQuery<AuditLogsResponse>({
    queryKey: auditQueryKeys.list(filterString),
    queryFn: async () => {
      const params = new URLSearchParams()
      if (methodFilter !== 'all') {
        params.set('method', methodFilter)
      }
      if (submittedPath) {
        params.set('path', submittedPath)
      }
      const data = (await api.getAdminAuditLogs(params)) as AuditLogsResponse
      return data ?? { items: [] }
    },
    staleTime: 10 * 1000,
  })

  function submitSearch(event: React.FormEvent) {
    event.preventDefault()
    setSubmittedPath(pathSearch.trim())
  }

  const items = auditQuery.data?.items ?? []

  return (
    <SettingsSectionCard
      title={t('settings.systemInfo.auditLogs.title')}
      description={t('settings.systemInfo.auditLogs.description')}
    >
      <form onSubmit={submitSearch} className='mb-4 flex flex-wrap items-center gap-3'>
        <Select value={methodFilter} onValueChange={(value) => setMethodFilter(value ?? 'all')}>
          <SelectTrigger className='w-32'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {METHOD_FILTERS.map((method) => (
              <SelectItem key={method} value={method}>
                {method === 'all'
                  ? t('settings.systemInfo.auditLogs.allMethods')
                  : method}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input
          value={pathSearch}
          onChange={(event) => setPathSearch(event.target.value)}
          placeholder={t('settings.systemInfo.auditLogs.pathPlaceholder')}
          className='flex-1'
        />
        <Button type='submit' variant='outline' size='sm'>
          {t('settings.systemInfo.auditLogs.search')}
        </Button>
      </form>
      {auditQuery.isLoading ? <SettingsSectionSkeleton /> : null}
      {!auditQuery.isLoading && items.length === 0 ? (
        <p className='py-8 text-center text-sm text-muted-foreground'>
          {t('settings.systemInfo.auditLogs.empty')}
        </p>
      ) : null}
      {!auditQuery.isLoading && items.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('settings.systemInfo.auditLogs.columns.time')}</TableHead>
              <TableHead>{t('settings.systemInfo.auditLogs.columns.method')}</TableHead>
              <TableHead>{t('settings.systemInfo.auditLogs.columns.path')}</TableHead>
              <TableHead>{t('settings.systemInfo.auditLogs.columns.status')}</TableHead>
              <TableHead>{t('settings.systemInfo.auditLogs.columns.actor')}</TableHead>
              <TableHead>{t('settings.systemInfo.auditLogs.columns.ip')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((entry) => (
              <TableRow key={entry.id}>
                <TableCell className='text-xs text-muted-foreground'>
                  {entry.createdAt}
                </TableCell>
                <TableCell>
                  <Badge variant={methodVariant(entry.method)}>{entry.method}</Badge>
                </TableCell>
                <TableCell className='font-mono text-xs'>{entry.path}</TableCell>
                <TableCell>
                  <span
                    className={
                      entry.status >= 400
                        ? 'text-destructive'
                        : 'text-success'
                    }
                  >
                    {entry.status}
                  </span>
                </TableCell>
                <TableCell className='text-xs'>{entry.actor ?? '—'}</TableCell>
                <TableCell className='text-xs text-muted-foreground'>
                  {entry.remoteIp ?? '—'}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : null}
    </SettingsSectionCard>
  )
}

function methodVariant(method: string): 'default' | 'secondary' | 'destructive' {
  if (method === 'DELETE') {
    return 'destructive'
  }
  if (method === 'POST' || method === 'PUT' || method === 'PATCH') {
    return 'default'
  }
  return 'secondary'
}
