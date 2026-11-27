import { Link, useNavigate } from '@tanstack/react-router'
import type { Row } from '@tanstack/react-table'
import { Activity, Pencil, Plus, Route, Settings2, Zap } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { PageHeader } from '@/components/common/page-header'
import { DataTablePage } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'

import { useRebuildRoutes, useRefreshRouteDecisions, useRoutes } from '../api'
import { useRouteRebuildTask } from '../lib/use-route-rebuild-task'
import type { RouteSummaryRow } from '../types'
import { resolveRouteTitle } from '../utils'
import { RouteChannelList } from './route-channel-list'
import { RouteDetailSheet } from './route-detail-sheet'
import { RouteFormDialog } from './route-form-dialog'
import { RouteRebuildStatus } from './route-rebuild-status'
import { RouteWorkspace, type RouteWorkspaceFilters } from './route-workspace'
import { RoutesBulkActions } from './routes-bulk-actions'
import { RoutesHeaderActions } from './routes-header-actions'
import { RoutesKeyNextStep } from './routes-key-next-step'
import { useRouteChainContext } from './use-route-chain-context'
import { useRouteDialogs } from './use-route-dialogs'
import { routeContentId, useRoutesTable } from './use-routes-table'
import { useTokenRoutesUrlState } from './use-token-routes-url-state'

export function RoutesPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const urlState = useTokenRoutesUrlState()
  const { filters, updateUrlState } = urlState

  const {
    data: routesData,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useRoutes()
  const rebuildMutation = useRebuildRoutes()
  const rebuildTask = useRouteRebuildTask()
  const isRebuildPending = rebuildMutation.isPending || rebuildTask.isBusy
  const refreshDecisionsMutation = useRefreshRouteDecisions()

  const routes = useMemo(() => routesData ?? [], [routesData])

  const { chainContext, chainAccountName, chainSiteName, accountOptions } =
    useRouteChainContext(filters)
  const { accountId, tokenId, siteId } = chainContext
  const {
    openCreate,
    openEdit,
    openDetail,
    openWorkspaceRoute,
    editDiagnosticRoute,
    formProps,
    detailProps,
  } = useRouteDialogs(routes, isLoading, filters.routeId)

  const [rebuildConfirmOpen, setRebuildConfirmOpen] = useState(false)

  const {
    table,
    expandedRouteIds,
    toggleRouteExpansion,
    showZeroChannel,
    setShowZeroChannel,
    deletionDialog,
  } = useRoutesTable({
    routes,
    urlState,
    openEdit,
    openDetail,
    refreshDecisions: refreshDecisionsMutation.mutate,
  })

  const openWorkspaceKeyLogs = useCallback(
    (keyId: number) => {
      void navigate({
        href: `/proxy-logs?downstreamKeyId=${keyId}`,
        replace: true,
      })
    },
    [navigate]
  )

  const openWorkspaceKeyEdit = useCallback(
    (keyId: number, locateField: string | null) => {
      const params = new URLSearchParams({ edit: String(keyId) })
      if (locateField) params.set('locateField', locateField)
      void navigate({
        href: `/downstream-keys?${params.toString()}`,
        replace: true,
      })
    },
    [navigate]
  )

  return (
    <div className='page-padding flex h-full flex-col gap-3'>
      <PageHeader
        title={t('tokenRoutes.page.title')}
        description={t('tokenRoutes.page.description')}
        actions={
          <RoutesHeaderActions
            onRebuild={() => setRebuildConfirmOpen(true)}
            isRebuildPending={isRebuildPending}
            onRefreshDecisions={() => refreshDecisionsMutation.mutate()}
            isRefreshDecisionsPending={refreshDecisionsMutation.isPending}
            onAddRoute={openCreate}
          />
        }
      >
        <Link
          to='/settings/$subarea/$section'
          params={{ subarea: 'operations', section: 'scheduling' }}
          className='text-primary focus-visible:ring-focus-ring mt-1 inline-flex min-h-8 items-center gap-1 rounded-sm text-sm underline-offset-4 hover:underline focus-visible:ring-3 focus-visible:outline-none'
        >
          <Settings2 className='size-3.5 shrink-0' aria-hidden='true' />
          {t('tokenRoutes.page.automaticCreationSettings')}
        </Link>
      </PageHeader>

      <RouteRebuildStatus rebuild={rebuildMutation} observation={rebuildTask} />

      <RouteWorkspace
        filters={{
          workspaceModel: filters.workspaceModel,
          workspaceEndpoint: filters.workspaceEndpoint,
          workspaceKeyId: filters.workspaceKeyId,
          workspaceClientIp: filters.workspaceClientIp,
        }}
        onFiltersChange={(next) =>
          updateUrlState({
            filters: next satisfies Partial<RouteWorkspaceFilters>,
          })
        }
        onOpenRoute={openWorkspaceRoute}
        onOpenKeyLogs={openWorkspaceKeyLogs}
        onOpenKeyEdit={openWorkspaceKeyEdit}
        onEditRoute={editDiagnosticRoute}
      />

      {(accountId || siteId) && (
        <div className='bg-muted/40 text-muted-foreground rounded-lg border p-2 text-sm'>
          {t('tokenRoutes.page.chainContext')}
          {chainAccountName
            ? ` ${t('tokenRoutes.page.chainContextAccount', { name: chainAccountName })}`
            : ''}
          {chainSiteName
            ? ` / ${t('tokenRoutes.page.chainContextSite', { name: chainSiteName })} `
            : ' '}
          {accountId && tokenId && (
            <span className='mr-2'>
              · {t('tokenRoutes.formPattern.selectedKey', { id: tokenId })}
            </span>
          )}
          {t('tokenRoutes.page.chainContextSuffix')}
        </div>
      )}

      {/* Journey step 3 → 4: mounted only when there is something to hand
          off. The component owns its own "no key issued yet" check and stays
          silent while that count is unknown, so this never adds a banner to
          the empty state or to a deployment that already has keys. */}
      {routes.length > 0 && <RoutesKeyNextStep />}

      <DataTablePage
        table={table}
        srOnlyHeader
        // An expanded channel table is the content the operator asked for.
        // Keeping the list in a fixed-height scroll region clipped the second
        // channel below the fold at 1024px, so let the page grow while any
        // route is open.
        fixedHeight={expandedRouteIds.size === 0}
        rowDisclosure={{
          isExpanded: (row: Row<RouteSummaryRow>) =>
            expandedRouteIds.has(row.original.id),
          onToggle: (row: Row<RouteSummaryRow>) =>
            toggleRouteExpansion(row.original.id),
          getContentId: (row: Row<RouteSummaryRow>) =>
            routeContentId(row.original.id),
          getLabel: (row: Row<RouteSummaryRow>) =>
            t('tokenRoutes.detail.inlineLabel', {
              model: resolveRouteTitle(row.original),
            }),
          renderContent: (row: Row<RouteSummaryRow>) => (
            <RouteChannelList
              route={row.original}
              actions={
                <>
                  {row.original.kind !== 'zero_channel' &&
                    !row.original.readOnly && (
                      <Button
                        variant='ghost'
                        size='icon-xs'
                        aria-label={t('common.edit')}
                        title={t('common.edit')}
                        onClick={() => openEdit(row.original)}
                      >
                        <Pencil />
                      </Button>
                    )}
                  <Button
                    variant='ghost'
                    size='icon-xs'
                    aria-label={t('tokenRoutes.detail.openDiagnostics')}
                    title={t('tokenRoutes.detail.openDiagnostics')}
                    onClick={() => openDetail(row.original)}
                  >
                    <Activity />
                  </Button>
                </>
              }
            />
          ),
        }}
        isLoading={isLoading}
        isFetching={isFetching}
        error={error as Error | null}
        errorMessageKey='tokenRoutes.page.loadError'
        onErrorRetry={() => refetch()}
        isErrorRetrying={isFetching}
        emptyIcon={<Route className='size-6' />}
        emptyTitle={t('tokenRoutes.page.emptyTitle')}
        emptyDescription={t('tokenRoutes.page.emptyDescription')}
        emptyAction={
          <div className='flex items-center gap-2'>
            <Button onClick={openCreate}>
              <Plus />
              {t('tokenRoutes.page.addButton')}
            </Button>
            <Button
              variant='outline'
              onClick={() => setRebuildConfirmOpen(true)}
              disabled={isRebuildPending}
            >
              {isRebuildPending ? <Spinner aria-hidden='true' /> : <Zap />}
              {t('tokenRoutes.page.rebuild')}
            </Button>
          </div>
        }
        skeletonKeyPrefix='routes-skeleton'
        toolbarProps={{
          searchPlaceholder: t('tokenRoutes.page.searchPlaceholder'),
          searchDebounceMs: 300,
          viewToggle: (
            // Short label + the long explanation on the tooltip: truncating
            // the full sentence clipped it mid-parenthesis, which read as a
            // rendering bug. The whole label row is the hit target (44px).
            <label
              htmlFor='show-zero-channel-routes'
              className='text-muted-foreground flex min-h-11 min-w-0 cursor-pointer items-center gap-1.5 text-sm'
              title={t('tokenRoutes.page.showZeroChannelHint')}
            >
              <Switch
                id='show-zero-channel-routes'
                className='shrink-0'
                checked={showZeroChannel}
                onCheckedChange={setShowZeroChannel}
              />
              <span className='min-w-0 truncate'>
                {t('tokenRoutes.page.showZeroChannel')}
              </span>
            </label>
          ),
          filters: [
            {
              columnId: 'enabled',
              title: t('tokenRoutes.page.filterStatusTitle'),
              singleSelect: true,
              options: [
                {
                  label: t('tokenRoutes.page.filterStatusEnabled'),
                  value: 'enabled',
                },
                {
                  label: t('tokenRoutes.page.filterStatusDisabled'),
                  value: 'disabled',
                },
              ],
            },
          ],
        }}
        bulkActions={<RoutesBulkActions table={table} />}
      />

      <RouteFormDialog
        {...formProps}
        accountOptions={accountOptions}
        chainContext={chainContext}
      />

      <RouteDetailSheet {...detailProps} />

      {deletionDialog}

      <ConfirmDialog
        open={rebuildConfirmOpen}
        title={t('tokenRoutes.page.rebuildConfirmTitle')}
        description={t('tokenRoutes.page.rebuildConfirmDescription')}
        confirmLabel={t('tokenRoutes.page.rebuild')}
        cancelLabel={t('common.cancel')}
        destructive
        onConfirm={() => {
          setRebuildConfirmOpen(false)
          if (!isRebuildPending) {
            rebuildMutation.mutate({ refreshModels: true, wait: false })
          }
        }}
        onCancel={() => setRebuildConfirmOpen(false)}
      />
    </div>
  )
}
