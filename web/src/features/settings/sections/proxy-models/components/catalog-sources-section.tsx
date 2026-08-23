// metapi-go/features/settings/sections/proxy-models — model-catalog data
// source registry panel (Wave 8 Lane A, reorder rework Wave 9 Lane B). Lists
// the DB-persisted catalog sources in merge order (earlier sources override
// later ones), exposes per-source enable/URL edit/drag-reorder/delete +
// "sync now", a global auto-sync toggle, and the merged-snapshot status.
// Reorder is a pointer-drag on the row handle (Wave 9 Lane B: the retired
// up/down arrow buttons); each drop writes the absolute target index through
// PUT /api/models/catalog-sources/{id} { sortOrder } (the backend repositions
// and renumbers contiguously). All mutations go through the
// /api/models/catalog-sources + /api/models/catalog-sync admin endpoints.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  GripVertical as GripVerticalIcon,
  Pencil as PencilIcon,
  Plus as PlusIcon,
  RefreshCw as RefreshCwIcon,
  Trash2 as Trash2Icon,
} from 'lucide-react'
import { useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toBcp47 } from '@/i18n/languages'
import { api, type CatalogSource, type CatalogSyncStatus } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'

const catalogSyncKeys = {
  all: ['catalog-sync'] as const,
  status: () => [...catalogSyncKeys.all, 'status'] as const,
}

type SourceDialogState =
  | { mode: 'new' }
  | { mode: 'edit'; source: CatalogSource }

function LastSyncCell({ source }: { source: CatalogSource }) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  if (!source.lastAttemptAt) {
    return (
      <span className='text-muted-foreground text-sm'>
        {t('settings.proxyModels.catalogSources.neverSynced')}
      </span>
    )
  }
  const at = formatDateTime(source.lastAttemptAt, locale)
  const count = t('settings.proxyModels.catalogSources.modelCount', {
    count: source.lastCount,
  })
  if (source.lastError) {
    return (
      <span className='flex flex-col'>
        <span className='text-sm'>{at}</span>
        <span
          className='text-destructive truncate text-xs'
          title={source.lastError}
        >
          {source.lastError}
        </span>
      </span>
    )
  }
  return (
    <span className='flex flex-col'>
      <span className='text-sm'>{at}</span>
      <span className='text-muted-foreground text-xs'>{count}</span>
    </span>
  )
}

export function CatalogSourcesSection() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const queryClient = useQueryClient()
  const [dialog, setDialog] = useState<SourceDialogState | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<CatalogSource | null>(null)
  const [syncingId, setSyncingId] = useState<number | 'all' | null>(null)

  // --- pointer-drag reorder state (Wave 9 Lane B) ---
  // Declared with the other hooks — BEFORE the loading/error early returns —
  // so the hook count stays stable across status states.
  const rowRefs = useRef<Array<HTMLTableRowElement | null>>([])
  const dragState = useRef<{ index: number } | null>(null)
  const overIndexRef = useRef<number | null>(null)
  const [dragIndex, setDragIndex] = useState<number | null>(null)
  const [overIndex, setOverIndex] = useState<number | null>(null)

  const statusQuery = useQuery<CatalogSyncStatus>({
    queryKey: catalogSyncKeys.status(),
    queryFn: async () => api.getCatalogSync(),
    staleTime: 10 * 1000,
  })

  const refreshStatus = (status: CatalogSyncStatus) => {
    queryClient.setQueryData(catalogSyncKeys.status(), status)
  }

  const syncAllMutation = useMutation({
    mutationFn: async () => api.syncCatalog(),
    onMutate: () => setSyncingId('all'),
    onSettled: () => setSyncingId(null),
    onSuccess: (status) => {
      refreshStatus(status)
      toast.success(
        t('settings.proxyModels.catalogSources.toast.syncSucceeded', {
          count: status.snapshot.models,
        })
      )
    },
    onError: (error: Error) =>
      toast.error(
        t('settings.proxyModels.catalogSources.toast.syncFailed', {
          message: error.message,
        })
      ),
  })

  const syncOneMutation = useMutation({
    mutationFn: async (id: number) => api.syncCatalog(id),
    onMutate: (id) => setSyncingId(id),
    onSettled: () => setSyncingId(null),
    onSuccess: (status) => {
      refreshStatus(status)
      toast.success(
        t('settings.proxyModels.catalogSources.toast.syncSucceeded', {
          count: status.snapshot.models,
        })
      )
    },
    onError: (error: Error) =>
      toast.error(
        t('settings.proxyModels.catalogSources.toast.syncFailed', {
          message: error.message,
        })
      ),
  })

  const autoSyncMutation = useMutation({
    mutationFn: async (enabled: boolean) =>
      api.updateCatalogSyncConfig(enabled),
    onSuccess: (status) => {
      refreshStatus(status)
      toast.success(
        status.autoSync
          ? t('settings.proxyModels.catalogSources.toast.autoSyncOn')
          : t('settings.proxyModels.catalogSources.toast.autoSyncOff')
      )
    },
    onError: (error: Error) =>
      toast.error(
        t('settings.proxyModels.catalogSources.toast.saveFailed', {
          message: error.message,
        })
      ),
  })

  const toggleEnabledMutation = useMutation({
    mutationFn: async (source: CatalogSource) =>
      api.updateCatalogSource(source.id, { enabled: !source.enabled }),
    onSuccess: (_result, source) => {
      const status = statusQuery.data
      if (status) {
        refreshStatus({
          ...status,
          sources: status.sources.map((row) =>
            row.id === source.id ? { ...row, enabled: !source.enabled } : row
          ),
        })
      }
      toast.success(t('settings.proxyModels.catalogSources.toast.updated'))
    },
    onError: (error: Error) =>
      toast.error(
        t('settings.proxyModels.catalogSources.toast.saveFailed', {
          message: error.message,
        })
      ),
  })

  const reorderMutation = useMutation({
    mutationFn: async ({
      sourceId,
      targetIndex,
    }: {
      sourceId: number
      targetIndex: number
    }) => api.updateCatalogSource(sourceId, { sortOrder: targetIndex }),
    // Optimistic move: splice the row to its drop position so the drag feels
    // immediate; the backend repositions + renumbers sort_order server-side.
    onMutate: async ({ sourceId, targetIndex }) => {
      const status = statusQuery.data
      if (!status) return undefined
      const rows = [...status.sources]
      const from = rows.findIndex((row) => row.id === sourceId)
      if (from < 0) return undefined
      const [moved] = rows.splice(from, 1)
      rows.splice(Math.min(targetIndex, rows.length), 0, moved)
      refreshStatus({ ...status, sources: rows })
      return status
    },
    onError: (error: Error, _vars, context) => {
      if (context) refreshStatus(context)
      toast.error(
        t('settings.proxyModels.catalogSources.toast.saveFailed', {
          message: error.message,
        })
      )
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => api.deleteCatalogSource(id),
    onSuccess: (_result, id) => {
      const status = statusQuery.data
      if (status) {
        refreshStatus({
          ...status,
          sources: status.sources.filter((row) => row.id !== id),
        })
      }
      toast.success(t('settings.proxyModels.catalogSources.toast.deleted'))
    },
    onError: (error: Error) =>
      toast.error(
        t('settings.proxyModels.catalogSources.toast.deleteFailed', {
          message: error.message,
        })
      ),
  })

  if (statusQuery.isLoading) {
    return <SettingsSectionSkeleton />
  }
  if (statusQuery.error) {
    return (
      <SettingsSectionCard
        title={t('settings.proxyModels.catalogSources.title')}
        description={t('settings.proxyModels.catalogSources.description')}
      >
        <p className='text-muted-foreground text-sm'>
          {t('settings.proxyModels.catalogSources.toast.disabled')}
        </p>
      </SettingsSectionCard>
    )
  }

  const status = statusQuery.data as CatalogSyncStatus
  const sources = status.sources ?? []
  const busy = syncingId !== null

  // --- pointer-drag reorder helpers (Wave 9 Lane B) ---
  // The row handle captures the pointer and tracks the row under the cursor;
  // on release the drop index is committed as an absolute sortOrder. Storing
  // the live target in a ref keeps the pointerup handler race-free even when
  // the last pointermove and pointerup land in the same frame.
  function findRowIndexAt(clientY: number): number {
    const rows = rowRefs.current
    for (let i = 0; i < rows.length; i++) {
      const row = rows[i]
      if (!row) continue
      if (clientY < row.getBoundingClientRect().bottom) return i
    }
    return Math.max(rows.length - 1, 0)
  }

  function onDragHandlePointerDown(
    event: ReactPointerEvent<HTMLButtonElement>,
    index: number
  ) {
    if (busy || reorderMutation.isPending) return
    dragState.current = { index }
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  function onDragHandlePointerMove(
    event: ReactPointerEvent<HTMLButtonElement>
  ) {
    const state = dragState.current
    if (!state) return
    const next = findRowIndexAt(event.clientY)
    setDragIndex(state.index)
    setOverIndex(next)
    overIndexRef.current = next
  }

  function onDragHandlePointerEnd(commit: boolean) {
    const state = dragState.current
    if (!state) return
    dragState.current = null
    setDragIndex(null)
    setOverIndex(null)
    const targetIndex = overIndexRef.current
    overIndexRef.current = null
    if (commit && targetIndex !== null && targetIndex !== state.index) {
      reorderMutation.mutate({
        sourceId: sources[state.index].id,
        targetIndex,
      })
    }
  }

  return (
    <div className='flex flex-col gap-4'>
      <SettingsSectionCard
        title={t('settings.proxyModels.catalogSources.title')}
        description={t('settings.proxyModels.catalogSources.description')}
        actions={
          <>
            <Button
              size='sm'
              variant='outline'
              onClick={() => setDialog({ mode: 'new' })}
            >
              <PlusIcon className='size-3.5' />
              {t('settings.proxyModels.catalogSources.add')}
            </Button>
            <Button
              size='sm'
              onClick={() => syncAllMutation.mutate()}
              disabled={busy}
            >
              {syncingId === 'all' ? (
                <Spinner />
              ) : (
                <RefreshCwIcon className='size-3.5' />
              )}
              {syncingId === 'all'
                ? t('settings.proxyModels.catalogSources.syncAllPending')
                : t('settings.proxyModels.catalogSources.syncAll')}
            </Button>
          </>
        }
      >
        <div className='flex flex-col gap-4'>
          <div className='flex items-center justify-between gap-4 rounded-md border p-3'>
            <div className='flex flex-col gap-0.5'>
              <Label htmlFor='catalog-auto-sync'>
                {t('settings.proxyModels.catalogSources.autoSync')}
              </Label>
              <p className='text-muted-foreground text-xs'>
                {t('settings.proxyModels.catalogSources.autoSyncHint')}
                {status.intervalMin > 0 ? ` (${status.intervalMin} min)` : ''}
              </p>
            </div>
            <Switch
              id='catalog-auto-sync'
              checked={status.autoSync}
              onCheckedChange={(checked) => autoSyncMutation.mutate(checked)}
              disabled={autoSyncMutation.isPending}
              aria-label={t('settings.proxyModels.catalogSources.autoSync')}
            />
          </div>

          <div className='rounded-md border p-3'>
            <h3 className='text-sm font-medium'>
              {t('settings.proxyModels.catalogSources.snapshot.title')}
            </h3>
            <dl className='mt-2 grid grid-cols-1 gap-2 text-sm sm:grid-cols-3'>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('settings.proxyModels.catalogSources.snapshot.source')}
                </dt>
                <dd className='truncate' title={status.snapshot.source}>
                  {status.snapshot.source || '—'}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('settings.proxyModels.catalogSources.snapshot.fetchedAt')}
                </dt>
                <dd>
                  {status.snapshot.fetchedAt
                    ? formatDateTime(status.snapshot.fetchedAt, locale)
                    : t('settings.proxyModels.catalogSources.snapshot.never')}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('settings.proxyModels.catalogSources.snapshot.models')}
                </dt>
                <dd className='tabular-nums'>{status.snapshot.models}</dd>
              </div>
            </dl>
          </div>

          {sources.length === 0 ? (
            <p className='text-muted-foreground py-4 text-center text-sm'>
              {t('settings.proxyModels.catalogSources.toast.disabled')}
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  {/* Drag-handle column (reorder, Wave 9 Lane B). */}
                  <TableHead className='w-7' />
                  <TableHead>
                    {t('settings.proxyModels.catalogSources.columns.source')}
                  </TableHead>
                  <TableHead>
                    {t('settings.proxyModels.catalogSources.columns.type')}
                  </TableHead>
                  <TableHead>
                    {t('settings.proxyModels.catalogSources.columns.url')}
                  </TableHead>
                  <TableHead>
                    {t('settings.proxyModels.catalogSources.columns.enabled')}
                  </TableHead>
                  <TableHead>
                    {t('settings.proxyModels.catalogSources.columns.lastSync')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('settings.proxyModels.catalogSources.columns.actions')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sources.map((source, index) => (
                  <TableRow
                    key={source.id}
                    ref={(row) => {
                      rowRefs.current[index] = row
                    }}
                    className={cn(
                      dragIndex !== null &&
                        overIndex === index &&
                        'bg-accent/60',
                      dragIndex === index && 'opacity-40'
                    )}
                  >
                    <TableCell className='w-7 px-0'>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        className={cn(
                          'cursor-grab touch-none select-none',
                          dragIndex === index && 'cursor-grabbing'
                        )}
                        disabled={busy || reorderMutation.isPending}
                        onPointerDown={(event) =>
                          onDragHandlePointerDown(event, index)
                        }
                        onPointerMove={onDragHandlePointerMove}
                        onPointerUp={() => onDragHandlePointerEnd(true)}
                        onPointerCancel={() => onDragHandlePointerEnd(false)}
                        aria-label={t(
                          'settings.proxyModels.catalogSources.dragToReorder'
                        )}
                        title={t(
                          'settings.proxyModels.catalogSources.dragToReorder'
                        )}
                      >
                        <GripVerticalIcon className='size-3.5' />
                      </Button>
                    </TableCell>
                    <TableCell className='text-sm font-medium'>
                      {source.name}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          source.type === 'official' ? 'secondary' : 'outline'
                        }
                      >
                        {source.type === 'official'
                          ? t(
                              'settings.proxyModels.catalogSources.typeOfficial'
                            )
                          : t('settings.proxyModels.catalogSources.typeCustom')}
                      </Badge>
                    </TableCell>
                    <TableCell
                      className='text-muted-foreground max-w-[16rem] truncate font-mono text-xs'
                      title={source.url}
                    >
                      {source.url}
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={source.enabled}
                        onCheckedChange={() =>
                          toggleEnabledMutation.mutate(source)
                        }
                        disabled={toggleEnabledMutation.isPending}
                        aria-label={t(
                          'settings.proxyModels.catalogSources.toggleEnabled',
                          {
                            name: source.name,
                          }
                        )}
                      />
                    </TableCell>
                    <TableCell>{<LastSyncCell source={source} />}</TableCell>
                    <TableCell>
                      <div className='flex justify-end gap-1'>
                        <Button
                          variant='ghost'
                          size='icon-sm'
                          disabled={busy}
                          onClick={() => syncOneMutation.mutate(source.id)}
                          aria-label={t(
                            'settings.proxyModels.catalogSources.syncNow'
                          )}
                        >
                          {syncingId === source.id ? (
                            <Spinner className='size-3.5' />
                          ) : (
                            <RefreshCwIcon className='size-3.5' />
                          )}
                        </Button>
                        <Button
                          variant='ghost'
                          size='icon-sm'
                          onClick={() => setDialog({ mode: 'edit', source })}
                          aria-label={t(
                            'settings.proxyModels.catalogSources.edit'
                          )}
                        >
                          <PencilIcon className='size-3.5' />
                        </Button>
                        <Button
                          variant='ghost'
                          size='icon-sm'
                          onClick={() => setDeleteTarget(source)}
                          aria-label={t(
                            'settings.proxyModels.catalogSources.delete'
                          )}
                        >
                          <Trash2Icon className='size-3.5' />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>
      </SettingsSectionCard>

      {dialog ? (
        <SourceDialog
          state={dialog}
          onOpenChange={(open) => {
            if (!open) setDialog(null)
          }}
          onSaved={() => {
            setDialog(null)
            void queryClient.invalidateQueries({
              queryKey: catalogSyncKeys.all,
            })
          }}
        />
      ) : null}

      <ConfirmDialog
        open={deleteTarget !== null}
        title={t('settings.proxyModels.catalogSources.deleteTitle')}
        description={t('settings.proxyModels.catalogSources.deleteDescription')}
        confirmLabel={t('settings.proxyModels.catalogSources.delete')}
        cancelLabel={t('settings.proxyModels.catalogSources.cancel')}
        destructive
        onConfirm={() => {
          if (deleteTarget) {
            deleteMutation.mutate(deleteTarget.id)
          }
          setDeleteTarget(null)
        }}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}

function SourceDialog({
  state,
  onOpenChange,
  onSaved,
}: {
  state: SourceDialogState
  onOpenChange: (open: boolean) => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const isEdit = state.mode === 'edit'
  const [name, setName] = useState(isEdit ? state.source.name : '')
  const [url, setUrl] = useState(isEdit ? state.source.url : '')
  const [type, setType] = useState<'official' | 'custom'>(
    isEdit ? state.source.type : 'custom'
  )
  const [enabled, setEnabled] = useState(isEdit ? state.source.enabled : true)

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (isEdit) {
        return api.updateCatalogSource(state.source.id, {
          name,
          url,
          type,
          enabled,
        })
      }
      return api.createCatalogSource({ name, url, type, enabled })
    },
    onSuccess: () => {
      toast.success(t('settings.proxyModels.catalogSources.toast.updated'))
      onSaved()
    },
    onError: (error: Error) =>
      toast.error(
        t('settings.proxyModels.catalogSources.toast.saveFailed', {
          message: error.message,
        })
      ),
  })

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>
            {isEdit
              ? t('settings.proxyModels.catalogSources.editTitle')
              : t('settings.proxyModels.catalogSources.addTitle')}
          </DialogTitle>
          <DialogDescription>
            {t('settings.proxyModels.catalogSources.description')}
          </DialogDescription>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='catalog-source-name'>
              {t('settings.proxyModels.catalogSources.nameLabel')}
            </Label>
            <Input
              id='catalog-source-name'
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder={t(
                'settings.proxyModels.catalogSources.namePlaceholder'
              )}
            />
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='catalog-source-url'>
              {t('settings.proxyModels.catalogSources.urlLabel')}
            </Label>
            <Input
              id='catalog-source-url'
              value={url}
              onChange={(event) => setUrl(event.target.value)}
              placeholder={t(
                'settings.proxyModels.catalogSources.urlPlaceholder'
              )}
            />
            <p className='text-muted-foreground text-xs'>
              {t('settings.proxyModels.catalogSources.urlHint')}
            </p>
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label>{t('settings.proxyModels.catalogSources.typeLabel')}</Label>
            <Select
              value={type}
              onValueChange={(value) => setType(value as 'official' | 'custom')}
            >
              <SelectTrigger className='w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='custom'>
                  {t('settings.proxyModels.catalogSources.typeCustom')}
                </SelectItem>
                <SelectItem value='official'>
                  {t('settings.proxyModels.catalogSources.typeOfficial')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className='flex items-center justify-between gap-2'>
            <Label htmlFor='catalog-source-enabled'>
              {t('settings.proxyModels.catalogSources.enabledLabel')}
            </Label>
            <Switch
              id='catalog-source-enabled'
              checked={enabled}
              onCheckedChange={setEnabled}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('settings.proxyModels.catalogSources.cancel')}
          </Button>
          <Button
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending || !name.trim() || !url.trim()}
          >
            {t('settings.proxyModels.catalogSources.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
