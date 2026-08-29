// metapi-go/features/settings/sections/downstream/components — tree picker
// over site → account → (default API key | tokens) for the downstream key
// credential-ref policy dimensions (allowedCredentialRefs /
// excludedCredentialRefs, #1026 UI follow-up). Contract SSOT: docs/api.md →
// Downstream API Keys → Credential & site scope. One picker instance is
// rendered per dimension; an empty selection means "unrestricted".
//
// Interaction model mirrors the Wave 17 SiteScopePicker: plain checkbox
// inputs (native keyboard support) inside a bordered scroll area, extended
// with collapsible site/account groups so the tree stays navigable on large
// fleets.

import { ChevronRight, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import {
  useAccounts,
  useAllAccountTokens,
  type Account,
  type AccountToken,
} from '@/features/accounts'
import { useSites, type Site } from '@/features/sites'
import { cn } from '@/lib/utils'

import { accountDisplayName, tokenDisplayName } from '../lib/credential-display'
import {
  credentialRefKey,
  serializeCredentialRefs,
  type CredentialRef,
} from '../lib/credential-refs'

type CredentialRefPickerProps = {
  value: CredentialRef[]
  onChange: (refs: CredentialRef[]) => void
}

/** Machine-readable description for refs whose upstream object is gone. */
function describeUnresolvedRef(ref: CredentialRef): string {
  const base = `${ref.kind} · siteId ${ref.siteId} · accountId ${ref.accountId}`
  return ref.kind === 'account_token'
    ? `${base} · tokenId ${ref.tokenId}`
    : base
}

export function CredentialRefPicker(props: CredentialRefPickerProps) {
  const { t } = useTranslation()
  const sitesQuery = useSites()
  const accountsQuery = useAccounts()
  const tokensQuery = useAllAccountTokens()

  const selectedKeys = useMemo(
    () => new Set(props.value.map(credentialRefKey)),
    [props.value]
  )

  // Sites (and token lists) holding a selection start expanded so edit mode
  // reveals the checked rows immediately. Mount-only state: the hosting form
  // remounts per sheet open, so a stale expansion set never leaks across
  // keys.
  const [expandedSiteIds, setExpandedSiteIds] = useState<Set<number>>(
    () => new Set(props.value.map((ref) => ref.siteId))
  )
  const [expandedTokenAccounts, setExpandedTokenAccounts] = useState<
    Set<number>
  >(
    () =>
      new Set(
        props.value
          .filter((ref) => ref.kind === 'account_token')
          .map((ref) => ref.accountId)
      )
  )

  const sites = useMemo(() => sitesQuery.data ?? [], [sitesQuery.data])

  const accountsBySite = useMemo(() => {
    const map = new Map<number, Account[]>()
    for (const account of accountsQuery.data?.accounts ?? []) {
      const list = map.get(account.siteId) ?? []
      list.push(account)
      map.set(account.siteId, list)
    }
    return map
  }, [accountsQuery.data])

  const tokensByAccount = useMemo(() => {
    const map = new Map<number, AccountToken[]>()
    for (const token of tokensQuery.data ?? []) {
      // accountId is nullish in the defensive token schema; rows without a
      // positive accountId cannot anchor a credential ref — skip them.
      const accountId = token.accountId
      if (accountId === null || accountId <= 0) continue
      const list = map.get(accountId) ?? []
      list.push(token)
      map.set(accountId, list)
    }
    return map
  }, [tokensQuery.data])

  function toggleSite(siteId: number) {
    setExpandedSiteIds((previous) => {
      const next = new Set(previous)
      if (next.has(siteId)) {
        next.delete(siteId)
      } else {
        next.add(siteId)
      }
      return next
    })
  }

  function toggleTokenList(accountId: number) {
    setExpandedTokenAccounts((previous) => {
      const next = new Set(previous)
      if (next.has(accountId)) {
        next.delete(accountId)
      } else {
        next.add(accountId)
      }
      return next
    })
  }

  function toggleRef(ref: CredentialRef, checked: boolean) {
    const key = credentialRefKey(ref)
    const next = props.value.filter((item) => credentialRefKey(item) !== key)
    if (checked) next.push(ref)
    props.onChange(serializeCredentialRefs(next))
  }

  if (
    sitesQuery.isPending ||
    accountsQuery.isPending ||
    tokensQuery.isPending
  ) {
    return (
      <div
        data-testid='credential-ref-picker'
        className='text-muted-foreground flex items-center gap-2 rounded-md border p-3 text-sm'
      >
        <Spinner />
        {t('settings.downstream.keys.credentials.loading')}
      </div>
    )
  }

  if (sitesQuery.isError || accountsQuery.isError || tokensQuery.isError) {
    return (
      <div
        data-testid='credential-ref-picker'
        className='text-destructive rounded-md border p-3 text-xs'
      >
        {t('settings.downstream.keys.credentials.loadFailed')}
      </div>
    )
  }

  // Dangling refs: stored refs whose site/account/token no longer exists.
  // They never match a candidate (a dangling allow ref fails closed) and the
  // backend rejects writing them back, so surface them for explicit removal
  // instead of hiding them inside the tree.
  const knownSiteIds = new Set(sites.map((site) => site.id))
  const knownAccountIds = new Set(
    (accountsQuery.data?.accounts ?? []).map((account) => account.id)
  )
  const knownTokenIds = new Set(
    (tokensQuery.data ?? []).map((token) => token.id)
  )
  const unresolvedRefs = props.value.filter((ref) => {
    if (!knownSiteIds.has(ref.siteId)) return true
    if (!knownAccountIds.has(ref.accountId)) return true
    return ref.kind === 'account_token' && !knownTokenIds.has(ref.tokenId)
  })

  function renderAccount(site: Site, account: Account) {
    const accountTokens = tokensByAccount.get(account.id) ?? []
    const tokensExpanded = expandedTokenAccounts.has(account.id)
    // The snapshot redacts apiToken to apiTokenMasked, so the masked field
    // is the honest "account has a default API key" indicator.
    const hasDefaultKey = Boolean(account.apiTokenMasked)
    const defaultRef: CredentialRef = {
      kind: 'default_api_key',
      siteId: site.id,
      accountId: account.id,
    }
    const accountName = accountDisplayName(account)

    return (
      <div key={account.id}>
        <div className='flex items-center gap-1'>
          {accountTokens.length > 0 ? (
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-expanded={tokensExpanded}
              aria-label={t(
                'settings.downstream.keys.credentials.expandTokensAria',
                { account: accountName }
              )}
              onClick={() => toggleTokenList(account.id)}
            >
              <ChevronRight
                aria-hidden='true'
                className={cn(
                  'transition-transform',
                  tokensExpanded && 'rotate-90'
                )}
              />
            </Button>
          ) : (
            <span aria-hidden='true' className='w-8 shrink-0' />
          )}
          <label className='flex min-w-0 flex-1 cursor-pointer items-center gap-2 py-1 text-sm'>
            <input
              type='checkbox'
              checked={selectedKeys.has(credentialRefKey(defaultRef))}
              disabled={!hasDefaultKey}
              onChange={(event) => toggleRef(defaultRef, event.target.checked)}
            />
            <span className='truncate'>{accountName}</span>
            <span className='text-muted-foreground shrink-0 text-xs'>
              {hasDefaultKey
                ? t('settings.downstream.keys.credentials.defaultApiKey')
                : t('settings.downstream.keys.credentials.noDefaultKey')}
            </span>
          </label>
        </div>
        {tokensExpanded
          ? accountTokens.map((token) => {
              const tokenRef: CredentialRef = {
                kind: 'account_token',
                siteId: site.id,
                accountId: account.id,
                tokenId: token.id,
              }
              return (
                <label
                  key={token.id}
                  className='text-muted-foreground ml-9 flex cursor-pointer items-center gap-2 py-0.5 text-xs'
                >
                  <input
                    type='checkbox'
                    checked={selectedKeys.has(credentialRefKey(tokenRef))}
                    onChange={(event) =>
                      toggleRef(tokenRef, event.target.checked)
                    }
                  />
                  <span className='truncate'>
                    {t('settings.downstream.keys.credentials.tokenPrefix')}{' '}
                    {tokenDisplayName(token)}
                  </span>
                </label>
              )
            })
          : null}
      </div>
    )
  }

  function renderSite(site: Site) {
    const siteAccounts = accountsBySite.get(site.id) ?? []
    const expanded = expandedSiteIds.has(site.id)
    const selectedCount = props.value.filter(
      (ref) => ref.siteId === site.id
    ).length

    return (
      <div key={site.id} className='border-border border-b last:border-b-0'>
        <button
          type='button'
          aria-expanded={expanded}
          aria-label={t('settings.downstream.keys.credentials.expandSiteAria', {
            site: site.name,
          })}
          onClick={() => toggleSite(site.id)}
          className='hover:bg-accent flex w-full items-center gap-2 px-2 py-1.5 text-sm'
        >
          <ChevronRight
            aria-hidden='true'
            className={cn(
              'shrink-0 transition-transform',
              expanded && 'rotate-90'
            )}
          />
          <span className='flex-1 truncate text-left'>{site.name}</span>
          {selectedCount > 0 ? (
            <Badge variant='secondary'>
              {t('settings.downstream.keys.credentials.selectedCount', {
                count: selectedCount,
              })}
            </Badge>
          ) : null}
        </button>
        {expanded ? (
          <div className='space-y-0.5 px-2 pb-2'>
            {siteAccounts.length === 0 ? (
              <p className='text-muted-foreground ps-9 text-xs'>
                {t('settings.downstream.keys.credentials.noAccounts')}
              </p>
            ) : (
              siteAccounts.map((account) => renderAccount(site, account))
            )}
          </div>
        ) : null}
      </div>
    )
  }

  return (
    <div
      data-testid='credential-ref-picker'
      className='border-border max-h-56 overflow-y-auto rounded-md border'
    >
      {sites.length === 0 ? (
        <p className='text-muted-foreground p-2 text-xs'>
          {t('settings.downstream.keys.credentials.noSites')}
        </p>
      ) : (
        sites.map(renderSite)
      )}
      {unresolvedRefs.length > 0 ? (
        <div className='border-border border-t p-2'>
          <p className='text-xs font-medium'>
            {t('settings.downstream.keys.credentials.unresolvedTitle')}
          </p>
          <ul className='mt-1 space-y-1'>
            {unresolvedRefs.map((ref) => (
              <li
                key={credentialRefKey(ref)}
                className='text-muted-foreground flex items-center gap-2 text-xs'
              >
                <span className='flex-1 truncate font-mono'>
                  {describeUnresolvedRef(ref)}
                </span>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  aria-label={t(
                    'settings.downstream.keys.credentials.unresolvedRemoveAria'
                  )}
                  onClick={() => toggleRef(ref, false)}
                >
                  <X aria-hidden='true' />
                </Button>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  )
}
