import { useQuery } from '@tanstack/react-query'
import { KeySquare } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  CredentialExportDialog,
  type CredentialExportTarget,
} from '@/components/common/credential-export-dialog'
import { SectionCard } from '@/components/common/section-card'
import { SectionError } from '@/components/common/section-error'
import { DataTablePage, useDataTable } from '@/components/data-table'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'
import { Sheet, SheetContent } from '@/components/ui/sheet'
import { api } from '@/lib/api'

import {
  downstreamKeysQueryKeys,
  type DownstreamApiKeyItem,
  type DownstreamKeysResponse,
} from '../types'
import {
  extractMarketplaceModelNames,
  resolveKeyScopeControl,
} from './key-form-shared'
import { KeyPermissionCheckDialog } from './key-permission-check-dialog'
import { KeySheetForm } from './key-sheet-form'
import { useKeyColumns } from './use-key-columns'
import { useKeyRestrictionFilter } from './use-key-restriction-filter'

type KeysSectionProps = {
  editId?: number
  initialLocateField?: string
  onEditDrilldownConsumed?: () => void
}

export function KeysSection({
  editId,
  initialLocateField,
  onEditDrilldownConsumed,
}: KeysSectionProps = {}) {
  const { t } = useTranslation()
  const [createOpen, setCreateOpen] = useState(false)
  const [formDirty, setFormDirty] = useState(false)
  // #1430: data-key-scope anchor of the restriction badge that opened the
  // edit sheet (null = opened from the plain edit action).
  const [locateField, setLocateField] = useState<string | null>(null)
  const [exportTarget, setExportTarget] =
    useState<CredentialExportTarget | null>(null)
  const [checkTarget, setCheckTarget] = useState<DownstreamApiKeyItem | null>(
    null
  )

  const keysQuery = useQuery<DownstreamKeysResponse>({
    queryKey: downstreamKeysQueryKeys.list(),
    queryFn: async () =>
      (await api.getDownstreamApiKeys()) as DownstreamKeysResponse,
    staleTime: 15 * 1000,
  })
  const migratedKey = keysQuery.data?.items.find(
    (item) => item.id === keysQuery.data?.migratedKeyId
  )

  const modelInventoryQuery = useQuery<unknown>({
    queryKey: [
      'models',
      'marketplace',
      { refresh: false, includePricing: false },
    ],
    queryFn: () => api.getModelsMarketplace(),
    enabled: createOpen,
    staleTime: 60 * 1000,
  })
  const candidateModels = useMemo(
    () => extractMarketplaceModelNames(modelInventoryQuery.data),
    [modelInventoryQuery.data]
  )

  const [editingKey, setEditingKey] = useState<DownstreamApiKeyItem | null>(
    null
  )

  function openCreate() {
    setEditingKey(null)
    setLocateField(null)
    setCreateOpen(true)
  }

  const openEdit = useCallback(
    (item: DownstreamApiKeyItem, locateField?: string | null) => {
      setEditingKey(item)
      setLocateField(locateField ?? null)
      setCreateOpen(true)
    },
    []
  )

  const editDrilldownConsumed = useRef(false)
  useEffect(() => {
    if (editId === undefined) {
      editDrilldownConsumed.current = false
      return
    }
    if (editDrilldownConsumed.current || keysQuery.isLoading) return

    const target = keysQuery.data?.items.find((item) => item.id === editId)
    editDrilldownConsumed.current = true
    if (target) openEdit(target, initialLocateField)
    onEditDrilldownConsumed?.()
  }, [
    editId,
    keysQuery.isLoading,
    keysQuery.data?.items,
    initialLocateField,
    onEditDrilldownConsumed,
    openEdit,
  ])

  function onSheetOpenChange(open: boolean) {
    setCreateOpen(open)
    if (!open) {
      setEditingKey(null)
      setLocateField(null)
    }
  }

  // User-initiated closes (X / Escape / overlay) are intercepted while the
  // form is dirty; the post-save close path calls onSheetOpenChange directly
  // on purpose so a successful submit never trips the discard prompt.
  const { handleOpenChange: guardedSheetOpenChange, guard: sheetDirtyGuard } =
    useDirtyDialogClose({
      enabled: formDirty,
      onDiscard: () => setFormDirty(false),
      onOpenChange: onSheetOpenChange,
    })

  const { items, toolbar } = useKeyRestrictionFilter(keysQuery.data?.items)
  const columns = useKeyColumns({ openEdit, setCheckTarget, setExportTarget })
  const { table } = useDataTable<DownstreamApiKeyItem>({
    data: items,
    columns,
    getRowId: (row) => String(row.id),
  })

  return (
    <SectionCard
      title={t('settings.downstream.keys.title')}
      description={t('settings.downstream.keys.description')}
      hideHeaderCopy
      actions={
        <Button size='sm' onClick={() => openCreate()}>
          {t('settings.downstream.keys.create')}
        </Button>
      }
    >
      {migratedKey && (
        <Notice tone='info' role='status' className='mb-3'>
          <KeySquare className='mt-0.5 size-4 shrink-0' />
          <div>
            <p className='font-medium'>{t('downstreamKeys.migration.title')}</p>
            <p>{t('downstreamKeys.migration.description')}</p>
            <Button
              variant='outline'
              size='sm'
              onClick={() => openEdit(migratedKey)}
              className='mt-2'
            >
              {t('downstreamKeys.migration.review')}
            </Button>
          </div>
        </Notice>
      )}
      {keysQuery.isError ? (
        <SectionError
          title={t('settings.downstream.keys.title')}
          messageKey='settings.downstream.keys.loadFailed'
          onRetry={() => void keysQuery.refetch()}
        />
      ) : (
        <DataTablePage
          table={table}
          isLoading={keysQuery.isLoading}
          emptyIcon={<KeySquare className='size-6' />}
          emptyTitle={t('settings.downstream.keys.empty')}
          emptyAction={
            <Button size='sm' onClick={() => openCreate()}>
              {t('settings.downstream.keys.create')}
            </Button>
          }
          toolbarProps={{ additionalSearch: toolbar }}
          fixedHeight={false}
          skeletonKeyPrefix='downstream-keys-skeleton'
        />
      )}
      {/* The mobile empty state carries no action button (MobileCardList
          contract) — the section header's Create action stays visible on
          every viewport, so the empty flow loses nothing. */}

      <Sheet open={createOpen} onOpenChange={guardedSheetOpenChange}>
        {/* Mobile contract comes from the SheetContent base: full-width panel
            below `sm` + flex-column layout. The form body is the scroll
            region (flex-1), so the submit footer (SheetFooter `mt-auto`)
            stays pinned at the bottom instead of scrolling out of reach. */}
        <SheetContent
          showMobileCloseBar={false}
          initialFocus={
            locateField
              ? () => resolveKeyScopeControl(locateField) ?? undefined
              : undefined
          }
        >
          <KeySheetForm
            key={editingKey?.id ?? 'create'}
            editingKey={editingKey}
            onDone={() => onSheetOpenChange(false)}
            onCreated={(target) => setExportTarget(target)}
            onDirtyChange={setFormDirty}
            candidateModels={candidateModels}
            initialLocateField={locateField}
          />
          {sheetDirtyGuard}
        </SheetContent>
      </Sheet>

      <CredentialExportDialog
        target={exportTarget}
        onOpenChange={(open) => {
          if (!open) setExportTarget(null)
        }}
      />

      <KeyPermissionCheckDialog
        keyId={checkTarget?.id ?? null}
        keyName={checkTarget?.name ?? ''}
        open={checkTarget !== null}
        onOpenChange={(open) => {
          if (!open) setCheckTarget(null)
        }}
      />
    </SectionCard>
  )
}
