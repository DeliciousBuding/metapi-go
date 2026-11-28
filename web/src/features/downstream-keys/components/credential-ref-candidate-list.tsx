import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { Account, AccountToken } from '@/features/accounts'
import type { Site } from '@/features/sites'
import { cn } from '@/lib/utils'

import { tokenDisplayName } from '../lib/credential-display'
import { credentialRefKey, type CredentialRef } from '../lib/credential-refs'
import {
  matchingAccount,
  type useCredentialRefExpansion,
} from './credential-ref-candidates'

type ToggleRef = (ref: CredentialRef, checked: boolean) => void

function AccountRow({
  row,
  selectedKeys,
  tokensExpanded,
  toggleTokenList,
  toggleRef,
}: {
  row: NonNullable<ReturnType<typeof matchingAccount>>
  selectedKeys: Set<string>
  tokensExpanded: boolean
  toggleTokenList: (accountId: number) => void
  toggleRef: ToggleRef
}) {
  const { t } = useTranslation()
  const {
    accountId,
    accountName,
    defaultRef,
    showDefault,
    hasDefaultKey,
    accountTokens,
    accountSelectedCount,
  } = row
  return (
    <div>
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
            onClick={() => toggleTokenList(accountId)}
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
        {showDefault ? (
          <label className='flex min-w-0 flex-1 cursor-pointer items-center gap-2 py-1 text-sm'>
            <input
              type='checkbox'
              checked={selectedKeys.has(credentialRefKey(defaultRef))}
              disabled={
                !hasDefaultKey &&
                !selectedKeys.has(credentialRefKey(defaultRef))
              }
              onChange={(event) => toggleRef(defaultRef, event.target.checked)}
            />
            <span className='truncate'>{accountName}</span>
            <span className='text-muted-foreground shrink-0 text-xs'>
              {hasDefaultKey
                ? t('settings.downstream.keys.credentials.defaultApiKey')
                : t('settings.downstream.keys.credentials.noDefaultKey')}
            </span>
          </label>
        ) : (
          <span className='min-w-0 flex-1 text-sm break-words'>
            {accountName}
          </span>
        )}
        {accountSelectedCount > 0 && (
          <Badge variant='secondary'>
            {t('settings.downstream.keys.picker.selected', {
              count: accountSelectedCount,
            })}
          </Badge>
        )}
      </div>
      {tokensExpanded
        ? accountTokens.map((token) => {
            const tokenRef: CredentialRef = {
              kind: 'account_token',
              siteId: defaultRef.siteId,
              accountId,
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

export function CredentialRefCandidateList({
  sites,
  accountsBySite,
  tokensByAccount,
  value,
  selectedKeys,
  search,
  onlySelected,
  expansion,
  toggleRef,
}: {
  sites: Site[]
  accountsBySite: Map<number, Account[]>
  tokensByAccount: Map<number, AccountToken[]>
  value: CredentialRef[]
  selectedKeys: Set<string>
  search: string
  onlySelected: boolean
  expansion: ReturnType<typeof useCredentialRefExpansion>
  toggleRef: ToggleRef
}) {
  const { t } = useTranslation()
  const query = search.trim().toLocaleLowerCase()
  const {
    expandedSiteIds,
    expandedTokenAccounts,
    toggleSite,
    toggleTokenList,
  } = expansion

  function renderSite(site: Site) {
    const siteAccounts = accountsBySite.get(site.id) ?? []
    const expanded =
      onlySelected || Boolean(query) || expandedSiteIds.has(site.id)
    const accountRows = siteAccounts
      .map((account) =>
        matchingAccount(
          site,
          account,
          tokensByAccount.get(account.id) ?? [],
          value,
          selectedKeys,
          onlySelected,
          query
        )
      )
      .filter((row) => row !== null)
    if (accountRows.length === 0 && (onlySelected || query)) return null
    const selectedCount = value.filter((ref) => ref.siteId === site.id).length

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
              accountRows.map((row) => (
                <AccountRow
                  key={row.accountId}
                  row={row}
                  selectedKeys={selectedKeys}
                  tokensExpanded={
                    onlySelected ||
                    Boolean(query) ||
                    expandedTokenAccounts.has(row.accountId)
                  }
                  toggleTokenList={toggleTokenList}
                  toggleRef={toggleRef}
                />
              ))
            )}
          </div>
        ) : null}
      </div>
    )
  }

  const siteRows = sites.map(renderSite).filter((row) => row !== null)
  return (
    <>
      {sites.length === 0 && (
        <p className='text-muted-foreground p-2 text-xs'>
          {t('settings.downstream.keys.credentials.noSites')}
        </p>
      )}
      {sites.length > 0 && siteRows.length > 0 && siteRows}
      {sites.length > 0 && siteRows.length === 0 && (
        <p className='text-muted-foreground p-2 text-xs'>
          {t('settings.downstream.keys.picker.noMatches')}
        </p>
      )}
    </>
  )
}
