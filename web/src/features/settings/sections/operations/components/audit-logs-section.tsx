// metapi-go/features/settings/sections/operations/components — admin audit
// logs section (B1). Read-only paginated table of admin write operations with
// method and path filters.

import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import { formatDateTime } from '@/lib/format'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'

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

type AuditLogsResponse = {
  items: AuditLogItem[]
  total: number
  limit: number
  offset: number
}

const PAGE_SIZE = 50

const auditQueryKeys = {
  all: ['admin-audit-logs'] as const,
  list: (filter: string) => [...auditQueryKeys.all, 'list', filter] as const,
}

const METHOD_FILTERS = ['all', 'POST', 'PUT', 'PATCH', 'DELETE'] as const

export function AuditLogsSection() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const [methodFilter, setMethodFilter] = useState<string>('all')
  const [pathSearch, setPathSearch] = useState('')
  const [submittedPath, setSubmittedPath] = useState('')
  const [page, setPage] = useState(0)

  const filterString = useMemo(() => {
    const params = new URLSearchParams()
    if (methodFilter !== 'all') params.set('method', methodFilter)
    if (submittedPath) params.set('path', submittedPath)
    params.set('limit', String(PAGE_SIZE))
    params.set('offset', String(page * PAGE_SIZE))
    return params.toString()
  }, [methodFilter, page, submittedPath])

  const auditQuery = useQuery<AuditLogsResponse>({
    queryKey: auditQueryKeys.list(filterString),
    queryFn: async () => {
      const data = (await api.getAdminAuditLogs(
        new URLSearchParams(filterString)
      )) as AuditLogsResponse
      return data ?? { items: [], total: 0, limit: PAGE_SIZE, offset: 0 }
    },
    staleTime: 10 * 1000,
  })

  function submitSearch(event: React.FormEvent) {
    event.preventDefault()
    setPage(0)
    setSubmittedPath(pathSearch.trim())
  }

  const items = auditQuery.data?.items ?? []
  const total = auditQuery.data?.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <SettingsSectionCard
      title={t('settings.operations.auditLogs.title')}
      description={t('settings.operations.auditLogs.description')}
    >
      <form
        onSubmit={submitSearch}
        className='mb-4 flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center'
      >
        <Select
          value={methodFilter}
          onValueChange={(value) => {
            setPage(0)
            setMethodFilter(value ?? 'all')
          }}
        >
          <SelectTrigger
            className='w-full sm:w-32'
            aria-label={t('settings.operations.auditLogs.columns.method')}
          >
            <SelectValue>
              {(selected) =>
                !selected || selected === 'all'
                  ? t('settings.operations.auditLogs.allMethods')
                  : String(selected)
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {METHOD_FILTERS.map((method) => (
              <SelectItem key={method} value={method}>
                {method === 'all'
                  ? t('settings.operations.auditLogs.allMethods')
                  : method}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input
          value={pathSearch}
          onChange={(event) => setPathSearch(event.target.value)}
          placeholder={t('settings.operations.auditLogs.pathPlaceholder')}
          className='min-w-0 flex-1'
        />
        <Button type='submit' variant='outline' size='sm'>
          {t('settings.operations.auditLogs.search')}
        </Button>
      </form>

      {auditQuery.isLoading ? <SettingsSectionSkeleton /> : null}
      {auditQuery.isError ? (
        <SettingsSectionError
          title={t('settings.operations.auditLogs.title')}
          onRetry={() => void auditQuery.refetch()}
        />
      ) : null}
      {!auditQuery.isLoading && !auditQuery.isError && items.length === 0 ? (
        <p className='text-muted-foreground py-8 text-center text-sm'>
          {t('settings.operations.auditLogs.empty')}
        </p>
      ) : null}
      {!auditQuery.isLoading && !auditQuery.isError && items.length > 0 ? (
        <>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  {t('settings.operations.auditLogs.columns.time')}
                </TableHead>
                <TableHead>
                  {t('settings.operations.auditLogs.columns.method')}
                </TableHead>
                <TableHead>
                  {t('settings.operations.auditLogs.columns.path')}
                </TableHead>
                <TableHead>
                  {t('settings.operations.auditLogs.columns.status')}
                </TableHead>
                <TableHead>
                  {t('settings.operations.auditLogs.columns.actor')}
                </TableHead>
                <TableHead>
                  {t('settings.operations.auditLogs.columns.ip')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((entry) => (
                <TableRow key={entry.id}>
                  <TableCell className='text-muted-foreground text-xs tabular-nums'>
                    {formatDateTime(entry.createdAt, locale)}
                  </TableCell>
                  <TableCell>
                    <Badge variant={methodVariant(entry.method)}>
                      {entry.method}
                    </Badge>
                  </TableCell>
                  <TableCell className='font-mono text-xs'>
                    {entry.path}
                  </TableCell>
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
                  <TableCell className='text-xs'>
                    {entry.actor ?? '—'}
                  </TableCell>
                  <TableCell className='text-muted-foreground text-xs'>
                    {entry.remoteIp ?? '—'}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <div className='text-muted-foreground mt-4 flex flex-col gap-2 border-t pt-3 text-xs sm:flex-row sm:items-center sm:justify-between'>
            <span>{t('settings.operations.auditLogs.total', { total })}</span>
            <div className='flex items-center gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={page === 0 || auditQuery.isFetching}
                onClick={() => setPage((current) => Math.max(0, current - 1))}
              >
                {t('settings.operations.auditLogs.previous')}
              </Button>
              <span>
                {t('settings.operations.auditLogs.page', {
                  current: page + 1,
                  total: pageCount,
                })}
              </span>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={page + 1 >= pageCount || auditQuery.isFetching}
                onClick={() => setPage((current) => current + 1)}
              >
                {t('settings.operations.auditLogs.next')}
              </Button>
            </div>
          </div>
        </>
      ) : null}
    </SettingsSectionCard>
  )
}

function methodVariant(
  method: string
): 'default' | 'secondary' | 'destructive' {
  if (method === 'DELETE') return 'destructive'
  if (method === 'POST' || method === 'PUT' || method === 'PATCH') {
    return 'default'
  }
  return 'secondary'
}
