import { useNavigate, useSearch } from '@tanstack/react-router'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import { ImportWizardDialog } from '@/features/import'

import { type AccountsPageData, useDeleteAccount } from '../api'
import {
  resolveDeepLinkCredentialMode,
  resolveDeepLinkPreselect,
} from '../lib/accounts-deep-link'
import type { Account, CredentialMode } from '../types'
import { AccountDetailSheet } from './account-detail-sheet'
import { AccountFormDialog } from './account-form-dialog'

export function useAccountsPageDialogs(
  accounts: Account[],
  {
    data,
    isLoading,
    refetch,
  }: {
    data: AccountsPageData | undefined
    isLoading: boolean
    refetch: () => Promise<unknown>
  }
) {
  const { t } = useTranslation()
  const search = useSearch({ from: '/_authenticated/accounts' })
  const navigate = useNavigate()
  const deleteMutation = useDeleteAccount()
  const [formOpen, setFormOpen] = useState(false)
  const [formMode, setFormMode] = useState<'create' | 'edit'>('create')
  const [editAccount, setEditAccount] = useState<Account | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailAccount, setDetailAccount] = useState<Account | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteAccount, setDeleteAccount] = useState<Account | null>(null)
  const [importOpen, setImportOpen] = useState(false)
  const [preselectedSiteId, setPreselectedSiteId] = useState<
    number | undefined
  >(undefined)
  const [preselectedCredentialMode, setPreselectedCredentialMode] = useState<
    CredentialMode | undefined
  >(undefined)

  const openCreate = () => {
    setFormMode('create')
    setEditAccount(null)
    setPreselectedSiteId(undefined)
    setPreselectedCredentialMode(undefined)
    setFormOpen(true)
  }

  // Consume the one-shot site → account deep link exactly once: resolve the
  // referenced site against the loaded snapshot, open the create dialog with
  // it preselected (apikey mode when the CTA passed the segment hint), then
  // strip the transient params from the URL so a refetch or remount never
  // reopens the dialog. Waits for the snapshot so a stale or unknown
  // `siteId` falls back safely instead of creating data. The snapshot query
  // is shared with the sites page (10s staleTime), so right after a guided
  // create the cached snapshot may still miss the new site — retry with a
  // refetch (bounded) before treating the id as unknown.
  const deepLinkConsumed = useRef(false)
  const deepLinkRefetchAttempts = useRef(0)
  useEffect(() => {
    if (deepLinkConsumed.current || search.create !== true) return
    if (isLoading || !data) return

    const stripDeepLink = () => {
      deepLinkConsumed.current = true
      navigate({
        to: '/accounts',
        search: {
          ...search,
          siteId: undefined,
          create: undefined,
          segment: undefined,
        },
        replace: true,
      })
    }

    const resolvedSiteId = resolveDeepLinkPreselect(
      search.create,
      search.siteId,
      data?.sites ?? []
    )
    if (resolvedSiteId !== null) {
      const resolvedMode = resolveDeepLinkCredentialMode(search.segment)
      setPreselectedSiteId(resolvedSiteId)
      setPreselectedCredentialMode(resolvedMode ?? undefined)
      setFormMode('create')
      setEditAccount(null)
      setFormOpen(true)
      stripDeepLink()
      return
    }

    // Site missing from the snapshot. Refetch once, a few times max, so a
    // just-created site resolves; an unknown stays silently consumed.
    if (deepLinkRefetchAttempts.current >= 3) {
      stripDeepLink()
      return
    }
    deepLinkRefetchAttempts.current += 1
    void refetch().catch(() => {
      // Fetch failures fall through; the next data change re-runs this
      // effect and eventually the retry bound consumes the link.
    })
  }, [search, isLoading, data, refetch, navigate])

  // Consume the one-shot attention deep link exactly once (dashboard
  // attention items link `/accounts?accountId=N` for expired / low-balance
  // accounts): resolve the referenced id against the loaded snapshot, open
  // the detail sheet with the same state the row "view detail" action uses,
  // then strip the transient param so a refetch or remount never reopens
  // the sheet. Waits for the snapshot so a stale or unknown id is cleared
  // silently instead of opening a blank sheet — mirrors the channels page's
  // `channelId` drilldown. The `useRef` guard survives the strict-mode
  // double-invoke and the post-navigate re-render.
  const accountDeepLinkConsumed = useRef(false)
  useEffect(() => {
    if (accountDeepLinkConsumed.current || !search.accountId) return
    if (isLoading) return

    const targetAccount = accounts.find(
      (account) => account.id === search.accountId
    )
    accountDeepLinkConsumed.current = true
    if (targetAccount) {
      setDetailAccount(targetAccount)
      setDetailOpen(true)
    }
    navigate({
      to: '/accounts',
      search: { ...search, accountId: undefined },
      replace: true,
    })
  }, [search, isLoading, accounts, navigate])

  const openEdit = useCallback((account: Account) => {
    setFormMode('edit')
    setEditAccount(account)
    setFormOpen(true)
  }, [])

  const openDelete = useCallback((account: Account) => {
    setDeleteAccount(account)
    setDeleteOpen(true)
  }, [])
  const openDetail = useCallback((account: Account) => {
    setDetailAccount(account)
    setDetailOpen(true)
  }, [])

  const confirmDelete = async () => {
    if (!deleteAccount) return
    try {
      await deleteMutation.mutateAsync(deleteAccount.id)
      setDeleteOpen(false)
      setDeleteAccount(null)
    } catch {
      // http-client toasted
    }
  }
  return {
    openCreate,
    openEdit,
    openDelete,
    openDetail,
    openImport: () => setImportOpen(true),
    dialogs: (
      <>
        <ImportWizardDialog open={importOpen} onOpenChange={setImportOpen} />

        {/* Create / edit form (Sheet) */}
        <AccountFormDialog
          open={formOpen}
          onOpenChange={setFormOpen}
          mode={formMode}
          account={editAccount}
          sites={data?.sites ?? []}
          initialSiteId={preselectedSiteId}
          initialCredentialMode={preselectedCredentialMode}
        />

        {/* Detail sheet (embeds the tokens sub-module) */}
        <AccountDetailSheet
          account={detailAccount}
          open={detailOpen}
          onOpenChange={setDetailOpen}
        />

        {/* Delete confirmation (Dialog) */}
        <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t('accounts.page.deleteTitle')}</DialogTitle>
              <DialogDescription>
                {t('accounts.page.deleteDescription', {
                  name: deleteAccount?.username || `#${deleteAccount?.id}`,
                })}
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button
                variant='outline'
                onClick={() => setDeleteOpen(false)}
                disabled={deleteMutation.isPending}
              >
                {t('common.cancel')}
              </Button>
              <Button
                variant='destructive'
                onClick={confirmDelete}
                disabled={deleteMutation.isPending}
              >
                {deleteMutation.isPending && <Spinner />}
                {t('common.delete')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </>
    ),
  }
}
