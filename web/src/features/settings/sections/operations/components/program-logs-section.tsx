// metapi-go/features/settings/sections/operations/components — program
// logs section. Operational event stream (GET /api/events) with type/unread
// filters, mark read / mark all read / clear, and a CSV export.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { api } from '@/lib/api'
import { neutralizeCsvFormulaCell } from '@/lib/helpers/csv-injection'
import { toast } from '@/lib/toast'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'
import {
  eventTitleKey,
  formatTimestamp,
  normalizeEvent,
  parseEventMessage,
  parsePanelPath,
  splitEnrichmentNames,
  type EventsResponse,
  type ProgramEvent,
} from '../lib/event-normalize'

const eventsQueryKeys = {
  all: ['program-events'] as const,
  list: (filter: string) => [...eventsQueryKeys.all, 'list', filter] as const,
}

const EVENT_TYPES = ['checkin', 'balance', 'token', 'proxy'] as const

function levelVariant(level?: string): 'default' | 'secondary' | 'destructive' {
  if (level === 'error') {
    return 'destructive'
  }
  if (level === 'warning') {
    return 'default'
  }
  return 'secondary'
}

/** Localized event title: known backend titles map to i18n keys, the rest render as-is. */
function EventTitle({ event }: { event: ProgramEvent }) {
  const { t } = useTranslation()
  const key = eventTitleKey(event.title)
  return key
    ? t(`settings.operations.programLogs.eventTitles.${key}`)
    : event.title
}

/**
 * Structured rendering of an event message. Enriched alert messages carry
 * "Affected routes / Alternative sites / Panel" lines appended by the backend;
 * those render as labeled rows with real SPA links instead of raw text.
 */
function EventMessage({ event }: { event: ProgramEvent }) {
  const { t } = useTranslation()
  if (!event.message) return null
  const parts = parseEventMessage(event.message)
  const routes = parts.routes ? splitEnrichmentNames(parts.routes) : []
  const sites = parts.sites ? splitEnrichmentNames(parts.sites) : []
  const panel =
    parts.panelPath !== null ? parsePanelPath(parts.panelPath) : null
  return (
    <span className='text-muted-foreground flex max-w-[360px] flex-col gap-0.5 text-xs'>
      {parts.base ? (
        <span className='line-clamp-2 break-all' title={parts.base}>
          {parts.base}
        </span>
      ) : null}
      {routes.length > 0 ? (
        <span className='flex flex-wrap items-baseline gap-x-1'>
          <span className='shrink-0'>
            {t('settings.operations.programLogs.messageParts.affectedRoutes')}:
          </span>
          {routes.map((name, index) => (
            <span key={name} className='inline-flex items-center gap-1'>
              <Link
                to='/token-routes'
                search={{ q: name }}
                className='text-primary underline-offset-2 hover:underline'
              >
                {name}
              </Link>
              {index < routes.length - 1 ? ',' : ''}
            </span>
          ))}
        </span>
      ) : null}
      {sites.length > 0 ? (
        <span className='flex flex-wrap items-baseline gap-x-1'>
          <span className='shrink-0'>
            {t('settings.operations.programLogs.messageParts.alternativeSites')}
            :
          </span>
          {sites.map((name, index) => (
            <span key={name} className='inline-flex items-center gap-1'>
              <Link
                to='/sites'
                search={{ q: name }}
                className='text-primary underline-offset-2 hover:underline'
              >
                {name}
              </Link>
              {index < sites.length - 1 ? ',' : ''}
            </span>
          ))}
        </span>
      ) : null}
      {panel ? <PanelLink panel={panel} /> : null}
    </span>
  )
}

/** Render a parsed panel path as a typed router link (known targets only). */
function PanelLink({ panel }: { panel: ReturnType<typeof parsePanelPath> }) {
  const { t } = useTranslation()
  if (!panel) return null
  const className =
    'text-primary inline-flex items-center gap-1 underline-offset-2 hover:underline'
  const label = t('settings.operations.programLogs.messageParts.openPanel')
  switch (panel.to) {
    case '/observability':
      return (
        <Link
          to='/observability'
          search={{
            section: (panel.search.section ?? 'health') as
              | 'overview'
              | 'health',
          }}
          className={className}
        >
          {label}
        </Link>
      )
    default:
      return null
  }
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
    <SettingsSectionCard
      title={t('settings.operations.programLogs.title')}
      description={t('settings.operations.programLogs.description')}
      actions={
        <div className='flex flex-wrap gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={markAllMutation.isPending}
            onClick={() => markAllMutation.mutate()}
          >
            {t('settings.operations.programLogs.markAllRead')}
          </Button>
          <Button type='button' variant='outline' size='sm' onClick={exportCsv}>
            {t('settings.operations.programLogs.exportCsv')}
          </Button>
          <Button
            type='button'
            variant='destructive'
            size='sm'
            disabled={clearMutation.isPending}
            onClick={() => setClearConfirmOpen(true)}
          >
            {t('settings.operations.programLogs.clear')}
          </Button>
        </div>
      }
    >
      <div className='mb-4 flex flex-wrap items-center gap-3'>
        <Select
          value={typeFilter}
          onValueChange={(value) => setTypeFilter(value ?? 'all')}
        >
          <SelectTrigger
            aria-label={t('settings.operations.programLogs.filterType')}
            className='w-40'
          >
            <SelectValue>
              {(selected) =>
                !selected || selected === 'all'
                  ? t('settings.operations.programLogs.allTypes')
                  : t(
                      `settings.operations.programLogs.type.${String(selected)}`
                    )
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>
              {t('settings.operations.programLogs.allTypes')}
            </SelectItem>
            {EVENT_TYPES.map((typeId) => (
              <SelectItem key={typeId} value={typeId}>
                {t(`settings.operations.programLogs.type.${typeId}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <label className='flex items-center gap-2 text-sm'>
          <Checkbox
            checked={unreadOnly}
            onCheckedChange={(checked) => setUnreadOnly(checked === true)}
          />
          {t('settings.operations.programLogs.unreadOnly')}
        </label>
        <span className='text-muted-foreground text-xs'>
          {t('settings.operations.programLogs.loadedCount', {
            count: items.length,
          })}
        </span>
      </div>
      {eventsQuery.isLoading ? <SettingsSectionSkeleton /> : null}
      {eventsQuery.isError ? (
        <SettingsSectionError
          title={t('settings.operations.programLogs.title')}
          onRetry={() => void eventsQuery.refetch()}
        />
      ) : null}
      {!eventsQuery.isLoading && !eventsQuery.isError && items.length === 0 ? (
        <p className='text-muted-foreground py-8 text-center text-sm'>
          {t('settings.operations.programLogs.empty')}
        </p>
      ) : null}
      {!eventsQuery.isLoading && !eventsQuery.isError && items.length > 0 ? (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                {t('settings.operations.programLogs.columns.time')}
              </TableHead>
              <TableHead>
                {t('settings.operations.programLogs.columns.level')}
              </TableHead>
              <TableHead>
                {t('settings.operations.programLogs.columns.type')}
              </TableHead>
              <TableHead>
                {t('settings.operations.programLogs.columns.title')}
              </TableHead>
              <TableHead className='text-right'>
                {t('settings.operations.programLogs.columns.actions')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((event) => (
              <TableRow key={event.id}>
                <TableCell className='text-muted-foreground text-xs'>
                  {formatTimestamp(event.createdAt)}
                </TableCell>
                <TableCell>
                  <Badge variant={levelVariant(event.level)}>
                    {t(
                      `settings.operations.programLogs.level.${event.level ?? 'info'}`
                    )}
                  </Badge>
                </TableCell>
                <TableCell className='text-xs'>
                  {t(
                    `settings.operations.programLogs.type.${event.type ?? ''}`,
                    { defaultValue: event.type ?? '—' }
                  )}
                </TableCell>
                <TableCell>
                  <div className='flex flex-col'>
                    <span
                      className={
                        event.read ? 'text-muted-foreground' : 'font-medium'
                      }
                    >
                      <EventTitle event={event} />
                    </span>
                    <EventMessage event={event} />
                  </div>
                </TableCell>
                <TableCell className='text-right'>
                  {event.read ? null : (
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      disabled={markReadMutation.isPending}
                      onClick={() => markReadMutation.mutate(event.id)}
                    >
                      {t('settings.operations.programLogs.markRead')}
                    </Button>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
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
    </SettingsSectionCard>
  )
}
