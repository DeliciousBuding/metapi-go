// metapi-go/features/settings/sections/downstream/components — downstream
// keys section. A lean list + create sheet + enable/disable/delete actions.
// The legacy DownstreamKeys page (1500+ lines, rich editor, batch ops, trend
// charts) is intentionally reduced to its core here; richer surfaces can be
// layered back on as separate sub-features once the rewrite matures.
//
// S8 teardown: the sheet form lives in key-sheet-form.tsx, the table cells
// in key-cells.tsx, and the shared types/schemas/mappers in
// key-form-shared.ts. The re-exports below keep the focused tests importing
// from this barrel.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { ColumnDef } from '@tanstack/react-table'
import { Pencil } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  CredentialExportDialog,
  type CredentialExportTarget,
} from '@/components/common/credential-export-dialog'
import { DataTablePage, useDataTable } from '@/components/data-table'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent } from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { useAccounts, useAllAccountTokens } from '@/features/accounts'
import { useSites } from '@/features/sites'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'
import { useUndoableDelete } from '@/lib/undoable-delete'

import { SettingsSectionCard } from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'
import { accountDisplayName, tokenDisplayName } from '../lib/credential-display'
import { KeyModelPolicyCell, KeyUsageCell } from './key-cells'
import {
  downstreamKeysQueryKeys,
  extractMarketplaceModelNames,
  type DownstreamApiKeyItem,
  type DownstreamKeysResponse,
} from './key-form-shared'
import { KeyScopeCell, type ScopeNameMaps } from './key-scope-cell'
import { KeySheetForm } from './key-sheet-form'

export { KeyModelPolicyCell, KeyUsageCell } from './key-cells'
export { KeySheetForm } from './key-sheet-form'

export function KeysSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [formDirty, setFormDirty] = useState(false)
  const [exportTarget, setExportTarget] =
    useState<CredentialExportTarget | null>(null)

  const keysQuery = useQuery<DownstreamKeysResponse>({
    queryKey: downstreamKeysQueryKeys.list(),
    queryFn: async () =>
      (await api.getDownstreamApiKeys()) as DownstreamKeysResponse,
    staleTime: 15 * 1000,
  })

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

  // Routing-scope display resolves raw site/account/token IDs to names.
  // These share their query keys with the sites/accounts/tokens surfaces
  // (and with the credential pickers inside the edit sheet), so the list
  // pays at most one extra fetch per inventory dimension.
  const scopeSitesQuery = useSites()
  const scopeAccountsQuery = useAccounts()
  const scopeTokensQuery = useAllAccountTokens()
  const scopeNames = useMemo<ScopeNameMaps>(
    () => ({
      sites: new Map(
        (scopeSitesQuery.data ?? []).map((site) => [site.id, site.name])
      ),
      accounts: new Map(
        (scopeAccountsQuery.data?.accounts ?? []).map((account) => [
          account.id,
          accountDisplayName(account),
        ])
      ),
      tokens: new Map(
        (scopeTokensQuery.data ?? []).map((token) => [
          token.id,
          tokenDisplayName(token),
        ])
      ),
    }),
    [scopeSitesQuery.data, scopeAccountsQuery.data, scopeTokensQuery.data]
  )

  const [editingKey, setEditingKey] = useState<DownstreamApiKeyItem | null>(
    null
  )

  function openCreate() {
    setEditingKey(null)
    setCreateOpen(true)
  }

  function openEdit(item: DownstreamApiKeyItem) {
    setEditingKey(item)
    setCreateOpen(true)
  }

  function onSheetOpenChange(open: boolean) {
    setCreateOpen(open)
    if (!open) {
      setEditingKey(null)
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

  const toggleMutation = useMutation({
    mutationFn: async ({ id, enabled }: { id: number; enabled: boolean }) =>
      api.updateDownstreamApiKey(id, { enabled }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: downstreamKeysQueryKeys.all,
      })
      toast.success(t('settings.downstream.keys.toast.updated'))
    },
    onError: () =>
      toast.error(t('settings.downstream.keys.toast.updateFailed')),
  })

  // S7 删除+undo 档: key revoke is leaf (no cascade) — no confirm dialog;
  // the row leaves immediately and a 6s undo toast gates the real DELETE.
  const undoableDelete = useUndoableDelete()
  const deleteKey = useCallback(
    (item: DownstreamApiKeyItem) =>
      undoableDelete<DownstreamKeysResponse, DownstreamApiKeyItem>({
        item,
        queryKey: downstreamKeysQueryKeys.list(),
        removeFromCache: (data, target) => ({
          ...data,
          items: data.items.filter((entry) => entry.id !== target.id),
        }),
        deleteFn: (target) => api.deleteDownstreamApiKey(target.id),
        title: t('settings.downstream.keys.toast.deleted'),
        undoLabel: t('common.undo'),
        errorTitle: t('settings.downstream.keys.toast.deleteFailed'),
      }),
    [undoableDelete, t]
  )

  const items = keysQuery.data?.items ?? []
  const isLoading = keysQuery.isLoading

  // Stable top-level refs for the columns memo (same pattern as the
  // accounts page's toggleStatusMutate): the mutation object's identity
  // changes every render, the memo only needs the stable mutate fn plus the
  // derived pending flag.
  const toggleKeyMutate = toggleMutation.mutate
  const toggleKeyPending = toggleMutation.isPending

  // One column set serves the desktop table and the ≤640px card list — the
  // same DataTablePage contract every list page uses: `mobileTitle` lifts
  // the key name into the card header, `mobileBadge` docks the enable
  // switch beside it, and the `actions` column id renders the
  // Connect/Edit/Delete buttons at the card bottom. Before this migration
  // the section rendered a bare <Table> that horizontally scrolled the
  // whole row set on phones, pushing Connect/Delete off-screen.
  const columns = useMemo<ColumnDef<DownstreamApiKeyItem, unknown>[]>(
    () => [
      {
        id: 'name',
        header: t('settings.downstream.keys.columns.name'),
        meta: { mobileTitle: true },
        cell: ({ row }) => (
          <div className='flex flex-col'>
            <span className='font-medium'>{row.original.name}</span>
            {row.original.keyMasked ? (
              <code className='text-muted-foreground text-xs'>
                {row.original.keyMasked}
              </code>
            ) : null}
          </div>
        ),
      },
      {
        id: 'group',
        header: t('settings.downstream.keys.columns.group'),
        cell: ({ row }) =>
          row.original.groupName ? (
            <Badge variant='secondary'>{row.original.groupName}</Badge>
          ) : (
            <span className='text-muted-foreground'>—</span>
          ),
      },
      {
        id: 'models',
        header: t('settings.downstream.keys.columns.models', {
          defaultValue: 'Models',
        }),
        cell: ({ row }) => (
          <KeyModelPolicyCell
            supportedModels={row.original.supportedModels}
            allowedRouteIds={row.original.allowedRouteIds}
          />
        ),
      },
      {
        id: 'scope',
        header: t('settings.downstream.keys.columns.scope'),
        cell: ({ row }) => (
          <KeyScopeCell item={row.original} names={scopeNames} />
        ),
      },
      {
        id: 'enabled',
        header: t('settings.downstream.keys.columns.enabled'),
        meta: { mobileBadge: true },
        cell: ({ row }) => (
          <Switch
            checked={row.original.enabled}
            disabled={toggleKeyPending}
            onCheckedChange={(checked) =>
              toggleKeyMutate({ id: row.original.id, enabled: checked })
            }
            aria-label={t('settings.downstream.keys.columns.enabledAria', {
              name: row.original.name,
            })}
          />
        ),
      },
      {
        id: 'usage',
        header: t('settings.downstream.keys.columns.usage'),
        cell: ({ row }) => (
          <div className='text-muted-foreground text-xs'>
            <KeyUsageCell item={row.original} />
          </div>
        ),
      },
      {
        id: 'actions',
        header: t('settings.downstream.keys.columns.actions'),
        cell: ({ row }) => (
          <div className='flex justify-end gap-1'>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() => setExportTarget(row.original)}
            >
              {t('settings.downstream.keys.connect')}
            </Button>
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label={t('settings.downstream.keys.columns.editAria', {
                name: row.original.name,
              })}
              onClick={() => openEdit(row.original)}
            >
              <Pencil />
            </Button>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() => deleteKey(row.original)}
            >
              {t('settings.common.delete')}
            </Button>
          </div>
        ),
      },
    ],
    [t, toggleKeyPending, toggleKeyMutate, scopeNames, deleteKey]
  )

  const { table } = useDataTable<DownstreamApiKeyItem>({
    data: items,
    columns,
    getRowId: (row) => String(row.id),
  })

  return (
    <SettingsSectionCard
      title={t('settings.downstream.keys.title')}
      description={t('settings.downstream.keys.description')}
      actions={
        <Button size='sm' onClick={() => openCreate()}>
          {t('settings.downstream.keys.create')}
        </Button>
      }
    >
      {keysQuery.isError ? (
        <SettingsSectionError
          title={t('settings.downstream.keys.title')}
          onRetry={() => void keysQuery.refetch()}
        />
      ) : (
        <DataTablePage
          table={table}
          columns={columns}
          isLoading={isLoading}
          emptyTitle={t('settings.downstream.keys.empty')}
          emptyAction={
            <Button size='sm' onClick={() => openCreate()}>
              {t('settings.downstream.keys.create')}
            </Button>
          }
          toolbarProps={null}
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
        <SheetContent showMobileCloseBar={false}>
          <KeySheetForm
            key={editingKey?.id ?? 'create'}
            editingKey={editingKey}
            onDone={() => onSheetOpenChange(false)}
            onCreated={(target) => setExportTarget(target)}
            onDirtyChange={setFormDirty}
            candidateModels={candidateModels}
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
    </SettingsSectionCard>
  )
}
