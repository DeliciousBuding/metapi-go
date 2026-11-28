// metapi-go/features/settings/sections/operations/components — program
// logs section. Operational event stream (GET /api/events) with type/unread
// filters, mark read / mark all read / clear, and a CSV export.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { SectionCard } from '@/components/common/section-card'
import { SectionError } from '@/components/common/section-error'
import { SectionSkeleton } from '@/components/common/section-skeleton'
import { EmptyText } from '@/components/ui/empty'
import { api } from '@/lib/api'
import { neutralizeCsvFormulaCell } from '@/lib/helpers/csv-injection'
import { toast } from '@/lib/toast'

import {
  normalizeEvent,
  type EventsResponse,
  type ProgramEvent,
} from '../lib/event-normalize'
import { ProgramLogsTable } from './program-logs-table'
import { ProgramLogActions, ProgramLogFilters } from './program-logs-toolbar'

const eventsQueryKeys = {
  all: ['program-events'] as const,
  list: (filter: string) => [...eventsQueryKeys.all, 'list', filter] as const,
}

export function ProgramLogsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [typeFilter, setTypeFilter] = useState<string>('all')
  const [unreadOnly, setUnreadOnly] = useState(false)
  const [clearConfirmOpen, setClearConfirmOpen] = useState(false)

  const filterQuery = useMemo(() => {
    const params = new URLSearchParams()
    if (typeFilter !== 'all') {
      params.set('type', typeFilter)
    }
    if (unreadOnly) {
      params.set('read', 'false')
    }
    params.set('limit', '50')
    return params.toString()
  }, [typeFilter, unreadOnly])

  const eventsQuery = useQuery<EventsResponse>({
    queryKey: eventsQueryKeys.list(filterQuery),
    queryFn: async () => {
      const data = (await api.getEvents(filterQuery)) as
        | EventsResponse
        | ProgramEvent[]
      const rawItems = Array.isArray(data) ? data : (data?.items ?? [])
      const items = rawItems.map((row) =>
        normalizeEvent(row as unknown as Record<string, unknown>)
      )
      if (Array.isArray(data)) {
        return { items, total: items.length, limit: 50 }
      }
      return { ...data, items, total: data?.total ?? items.length }
    },
    staleTime: 10 * 1000,
  })

  const markReadMutation = useMutation({
    mutationFn: async (id: number) => api.markEventRead(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: eventsQueryKeys.all })
    },
    onError: () =>
      toast.error(t('settings.operations.programLogs.toast.markReadFailed')),
  })

  const markAllMutation = useMutation({
    mutationFn: async () => api.markAllEventsRead(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: eventsQueryKeys.all })
      toast.success(t('settings.operations.programLogs.toast.allMarkedRead'))
    },
    onError: () =>
      toast.error(t('settings.operations.programLogs.toast.markAllFailed')),
  })

  const clearMutation = useMutation({
    mutationFn: async () => api.clearEvents(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: eventsQueryKeys.all })
      toast.success(t('settings.operations.programLogs.toast.cleared'))
    },
    onError: () =>
      toast.error(t('settings.operations.programLogs.toast.clearFailed')),
  })

  function exportCsv() {
    const events = eventsQuery.data?.items ?? []
    const header = 'time,level,type,title,message'
    const rows = events.map((event) =>
      [
        event.createdAt ?? '',
        event.level ?? '',
        event.type,
        event.title,
        event.message ?? '',
      ]
        .map(
          (field) =>
            `"${neutralizeCsvFormulaCell(String(field)).replaceAll('"', '""')}"`
        )
        .join(',')
    )
    const csv = [header, ...rows].join('\n')
    const blob = new Blob([csv], { type: 'text/csv' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = 'metapi-events.csv'
    anchor.click()
    URL.revokeObjectURL(url)
    toast.success(
      t('settings.operations.programLogs.toast.exported', {
        count: events.length,
      })
    )
  }

  const items = eventsQuery.data?.items ?? []

  return (
    <SectionCard
      title={t('settings.operations.programLogs.title')}
      description={t('settings.operations.programLogs.description')}
      hideHeaderCopy
      actions={
        <ProgramLogActions
          markingAllRead={markAllMutation.isPending}
          clearing={clearMutation.isPending}
          onMarkAllRead={() => markAllMutation.mutate()}
          onExport={exportCsv}
          onClear={() => setClearConfirmOpen(true)}
        />
      }
    >
      <ProgramLogFilters
        typeFilter={typeFilter}
        setTypeFilter={setTypeFilter}
        unreadOnly={unreadOnly}
        setUnreadOnly={setUnreadOnly}
        count={items.length}
      />
      {eventsQuery.isLoading ? <SectionSkeleton /> : null}
      {eventsQuery.isError ? (
        <SectionError
          title={t('settings.operations.programLogs.title')}
          messageKey='settings.common.loadFailed'
          onRetry={() => void eventsQuery.refetch()}
        />
      ) : null}
      {!eventsQuery.isLoading && !eventsQuery.isError && items.length === 0 ? (
        <EmptyText>{t('settings.operations.programLogs.empty')}</EmptyText>
      ) : null}
      {!eventsQuery.isLoading && !eventsQuery.isError && items.length > 0 ? (
        <ProgramLogsTable
          items={items}
          markingRead={markReadMutation.isPending}
          onMarkRead={(id) => markReadMutation.mutate(id)}
        />
      ) : null}

      <ConfirmDialog
        open={clearConfirmOpen}
        title={t('settings.operations.programLogs.clearTitle')}
        description={t('settings.operations.programLogs.clearDescription')}
        confirmLabel={t('settings.operations.programLogs.clear')}
        cancelLabel={t('settings.common.cancel')}
        destructive
        onConfirm={() => {
          setClearConfirmOpen(false)
          clearMutation.mutate()
        }}
        onCancel={() => setClearConfirmOpen(false)}
      />
    </SectionCard>
  )
}
