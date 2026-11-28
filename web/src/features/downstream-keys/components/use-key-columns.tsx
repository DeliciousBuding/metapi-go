import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { Pencil, ScrollText } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { useAccounts, useAllAccountTokens } from '@/features/accounts'
import { useSites } from '@/features/sites'

import { accountDisplayName, tokenDisplayName } from '../lib/credential-display'
import type { DownstreamApiKeyItem } from '../types'
import { KeyModelPolicyCell, KeyUsageCell } from './key-cells'
import { KeyScopeCell, type ScopeNameMaps } from './key-scope-cell'
import { useKeyRowActions } from './use-key-row-actions'

type KeyActions = {
  openEdit: (item: DownstreamApiKeyItem, locateField?: string | null) => void
  setExportTarget: (item: DownstreamApiKeyItem) => void
  setCheckTarget: (item: DownstreamApiKeyItem) => void
}

export function useKeyColumns({
  openEdit,
  setExportTarget,
  setCheckTarget,
}: KeyActions) {
  const { t } = useTranslation()
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
  const { toggleKeyMutate, toggleKeyPending, deleteKey } = useKeyRowActions()
  return useMemo<ColumnDef<DownstreamApiKeyItem, unknown>[]>(
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
          <KeyScopeCell
            item={row.original}
            names={scopeNames}
            onOpenDetails={(anchor) => openEdit(row.original, anchor)}
          />
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
              size='sm'
              onClick={() => setCheckTarget(row.original)}
            >
              {t('settings.downstream.keys.check.action')}
            </Button>
            <Button
              variant='ghost'
              size='icon-sm'
              aria-label={t('settings.downstream.keys.viewLogsAria', {
                name: row.original.name,
              })}
              title={t('settings.downstream.keys.viewLogs')}
              render={
                <Link
                  to='/proxy-logs'
                  search={{ downstreamKeyId: row.original.id }}
                />
              }
            >
              <ScrollText />
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
    [
      t,
      toggleKeyPending,
      toggleKeyMutate,
      scopeNames,
      deleteKey,
      openEdit,
      setExportTarget,
      setCheckTarget,
    ]
  )
}
