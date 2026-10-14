import { useQuery } from '@tanstack/react-query'
import { ChevronDown, Route, Search } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import {
  downstreamKeysQueryKeys,
  type DownstreamKeysResponse,
  KeyContextAdmissionResult,
} from '@/features/downstream-keys'
import { api, type KeyContextDecisionResponse } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'

import { routeCheckEndpoints } from '../lib/route-check-endpoints'
import type { RouteDecision } from '../types'
import { RouteDecisionResult } from './route-decision-result'

export type RouteWorkspaceFilters = {
  workspaceModel: string
  workspaceEndpoint: string
  workspaceKeyId: string
  workspaceClientIp: string
}

type RouteWorkspaceProps = {
  filters: RouteWorkspaceFilters
  onFiltersChange: (filters: Partial<RouteWorkspaceFilters>) => void
  onOpenRoute: (routeId: number) => void
  onOpenKeyLogs: (keyId: number) => void
}

export function RouteWorkspace({
  filters,
  onFiltersChange,
  onOpenRoute,
  onOpenKeyLogs,
}: RouteWorkspaceProps) {
  const { t } = useTranslation()
  const [model, setModel] = useState(filters.workspaceModel || '')
  const [endpoint, setEndpoint] = useState(
    filters.workspaceEndpoint || routeCheckEndpoints[0][0]
  )
  const [keyId, setKeyId] = useState(filters.workspaceKeyId || 'none')
  const [clientIp, setClientIp] = useState(filters.workspaceClientIp || '')
  const [open, setOpen] = useState(Boolean(filters.workspaceModel))

  useEffect(() => {
    setModel(filters.workspaceModel || '')
  }, [filters.workspaceModel])

  useEffect(() => {
    setEndpoint(filters.workspaceEndpoint || routeCheckEndpoints[0][0])
  }, [filters.workspaceEndpoint])

  useEffect(() => {
    setKeyId(filters.workspaceKeyId || 'none')
  }, [filters.workspaceKeyId])

  useEffect(() => {
    setClientIp(filters.workspaceClientIp || '')
  }, [filters.workspaceClientIp])

  const keysQuery = useQuery({
    queryKey: downstreamKeysQueryKeys.list(),
    queryFn: async () =>
      (await api.getDownstreamApiKeys()) as DownstreamKeysResponse,
    staleTime: 60 * 1000,
  })

  const submittedModel = (filters.workspaceModel || '').trim()
  const submittedEndpoint =
    filters.workspaceEndpoint || routeCheckEndpoints[0][0]
  const selectedKeyId =
    filters.workspaceKeyId && filters.workspaceKeyId !== 'none'
      ? Number(filters.workspaceKeyId)
      : null
  const submittedClientIp = (filters.workspaceClientIp || '').trim()

  const adminQuery = useQuery({
    queryKey: [
      'routes',
      'workspace',
      'admin',
      submittedModel,
      submittedEndpoint,
    ],
    queryFn: async () => {
      const response = await api.getRouteDecision({
        model: submittedModel,
        endpoint: submittedEndpoint,
      })
      return assertBusinessOk<{ decision: RouteDecision }>(
        response,
        'tokenRoutes.preview.failed'
      ).decision
    },
    enabled: submittedModel !== '' && selectedKeyId === null,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  const keyContextQuery = useQuery({
    queryKey: [
      'routes',
      'workspace',
      'key-context',
      submittedModel,
      submittedEndpoint,
      selectedKeyId,
      submittedClientIp,
    ],
    queryFn: async () => {
      if (selectedKeyId === null) throw new Error('no key selected')
      const response = await api.getRouteDecisionKeyContext({
        keyId: selectedKeyId,
        model: submittedModel,
        endpoint: submittedEndpoint,
        clientIp: submittedClientIp || undefined,
      })
      return assertBusinessOk<KeyContextDecisionResponse>(
        response,
        'tokenRoutes.preview.failed'
      )
    },
    enabled: submittedModel !== '' && selectedKeyId !== null,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })

  const activeQuery = selectedKeyId === null ? adminQuery : keyContextQuery
  const decision =
    selectedKeyId === null ? adminQuery.data : keyContextQuery.data?.decision
  const keyContext = selectedKeyId === null ? undefined : keyContextQuery.data
  const isFetching = activeQuery.isFetching
  const isError = activeQuery.isError

  const keyItems = useMemo(() => {
    const items = (keysQuery.data?.items ?? []).map((key) => ({
      value: String(key.id),
      label: key.name || `#${key.id}`,
    }))
    return [
      { value: 'none', label: t('tokenRoutes.workspace.noKey') },
      ...items,
    ]
  }, [keysQuery.data?.items, t])

  const submit = () => {
    onFiltersChange({
      workspaceModel: model.trim(),
      workspaceEndpoint: endpoint,
      workspaceKeyId: keyId,
      workspaceClientIp: clientIp.trim(),
    })
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className='rounded-lg border'
    >
      <CollapsibleTrigger className='group focus-visible:outline-ring flex w-full items-center justify-between gap-3 rounded-lg px-3 py-2 text-left text-sm font-medium focus-visible:outline-2 focus-visible:outline-offset-2'>
        <span className='flex min-w-0 items-center gap-2'>
          <Route className='size-4 shrink-0' aria-hidden='true' />
          <span className='min-w-0 truncate'>
            {t('tokenRoutes.workspace.title')}
          </span>
        </span>
        <ChevronDown
          className='size-4 shrink-0 transition-transform group-data-[panel-open]:rotate-180'
          aria-hidden='true'
        />
      </CollapsibleTrigger>
      <CollapsibleContent className='space-y-3 border-t p-3'>
        <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
          <div className='space-y-1.5'>
            <Label htmlFor='route-workspace-model'>
              {t('tokenRoutes.workspace.model')}
            </Label>
            <Input
              id='route-workspace-model'
              value={model}
              placeholder='gpt-4o'
              onChange={(event) => setModel(event.target.value)}
            />
          </div>
          <div className='space-y-1.5'>
            <Label>{t('tokenRoutes.workspace.endpoint')}</Label>
            <Select
              items={routeCheckEndpoints.map(([value, label]) => ({
                value,
                label,
              }))}
              value={endpoint}
              onValueChange={(value) => setEndpoint(value ?? endpoint)}
            >
              <SelectTrigger className='w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {routeCheckEndpoints.map(([value, label]) => (
                  <SelectItem key={value} value={value}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className='space-y-1.5'>
            <Label>{t('tokenRoutes.workspace.key')}</Label>
            <Select
              items={keyItems}
              value={keyId}
              onValueChange={(value) => setKeyId(value ?? 'none')}
            >
              <SelectTrigger className='w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {keyItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='route-workspace-client-ip'>
              {t('tokenRoutes.workspace.clientIp')}
            </Label>
            <Input
              id='route-workspace-client-ip'
              value={clientIp}
              placeholder='203.0.113.10'
              disabled={keyId === 'none'}
              onChange={(event) => setClientIp(event.target.value)}
            />
          </div>
        </div>

        <div className='flex flex-wrap items-center justify-between gap-2'>
          <p className='text-muted-foreground text-xs'>
            {selectedKeyId === null
              ? t('tokenRoutes.workspace.adminBoundary')
              : t('tokenRoutes.workspace.keyBoundary')}
          </p>
          <div className='flex items-center gap-2'>
            {isFetching ? (
              <Spinner className='size-3.5' aria-hidden='true' />
            ) : null}
            <Button size='sm' variant='outline' onClick={submit}>
              <Search className='size-4' />
              {t('tokenRoutes.workspace.check')}
            </Button>
          </div>
        </div>

        {isError ? (
          <p role='alert' className='text-destructive text-xs'>
            {t('tokenRoutes.workspace.error')}
          </p>
        ) : null}

        {submittedModel === '' ? (
          <p className='text-muted-foreground text-xs'>
            {t('tokenRoutes.workspace.empty')}
          </p>
        ) : null}

        {keyContext ? <KeyContextAdmissionResult result={keyContext} /> : null}

        {decision ? (
          <div className='space-y-2 border-t pt-3'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <h3 className='text-sm font-medium'>
                {t('tokenRoutes.workspace.routeResult')}
              </h3>
              <div className='flex flex-wrap gap-2'>
                {decision.routeId ? (
                  <Button
                    size='xs'
                    variant='outline'
                    onClick={() => onOpenRoute(decision.routeId as number)}
                  >
                    {t('tokenRoutes.workspace.openRoute')}
                  </Button>
                ) : null}
                {selectedKeyId !== null ? (
                  <Button
                    size='xs'
                    variant='outline'
                    onClick={() => onOpenKeyLogs(selectedKeyId)}
                  >
                    {t('tokenRoutes.workspace.openLogs')}
                  </Button>
                ) : null}
              </div>
            </div>
            <RouteDecisionResult decision={decision} />
          </div>
        ) : null}
      </CollapsibleContent>
    </Collapsible>
  )
}
