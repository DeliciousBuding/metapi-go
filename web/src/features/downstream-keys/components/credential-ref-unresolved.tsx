import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import type { Account, AccountToken } from '@/features/accounts'
import type { Site } from '@/features/sites'

import { credentialRefKey, type CredentialRef } from '../lib/credential-refs'

/** Machine-readable description for refs whose upstream object is gone. */
function describeUnresolvedRef(ref: CredentialRef): string {
  const base = `${ref.kind} · siteId ${ref.siteId} · accountId ${ref.accountId}`
  return ref.kind === 'account_token'
    ? `${base} · tokenId ${ref.tokenId}`
    : base
}

export function UnresolvedCredentialRefs({
  value,
  sites,
  accounts,
  tokens,
  onRemove,
}: {
  value: CredentialRef[]
  sites: Site[]
  accounts: Account[]
  tokens: AccountToken[]
  onRemove: (ref: CredentialRef) => void
}) {
  const { t } = useTranslation()
  // Dangling refs: stored refs whose site/account/token no longer exists.
  // They never match a candidate (a dangling allow ref fails closed) and the
  // backend rejects writing them back, so surface them for explicit removal
  // instead of hiding them inside the tree.
  const knownSiteIds = new Set(sites.map((site) => site.id))
  const knownAccountIds = new Set(accounts.map((account) => account.id))
  const knownTokenIds = new Set(tokens.map((token) => token.id))
  const unresolvedRefs = value.filter((ref) => {
    if (!knownSiteIds.has(ref.siteId)) return true
    if (!knownAccountIds.has(ref.accountId)) return true
    return ref.kind === 'account_token' && !knownTokenIds.has(ref.tokenId)
  })
  if (!unresolvedRefs.length) return null
  return (
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
              data-unresolved-ref
              aria-label={t(
                'settings.downstream.keys.credentials.unresolvedRemoveAria'
              )}
              onClick={() => onRemove(ref)}
            >
              <X aria-hidden='true' />
            </Button>
          </li>
        ))}
      </ul>
    </div>
  )
}
