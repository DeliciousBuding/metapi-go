// metapi-go/features/downstream-keys/components — tree picker
// over site → account → (default API key | tokens) for the downstream key
// credential-ref policy dimensions (allowedCredentialRefs /
// excludedCredentialRefs, #1026 UI follow-up). Contract SSOT: docs/api.md →
// Downstream API Keys → Credential & site scope. One picker instance is
// rendered per dimension; an empty selection means "unrestricted".
//
// Interaction model mirrors SiteScopePicker: plain checkbox
// inputs (native keyboard support) inside a bordered scroll area, extended
// with collapsible site/account groups so the tree stays navigable on large
// fleets.

import { useMemo, useState, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { formLabelIdFor } from '@/components/ui/form'
import { Spinner } from '@/components/ui/spinner'
import {
  useAccounts,
  useAllAccountTokens,
  type Account,
  type AccountToken,
} from '@/features/accounts'
import { useSites } from '@/features/sites'

import { useScopeSelectionFocus } from '../hooks/use-scope-selection-focus'
import {
  credentialRefKey,
  serializeCredentialRefs,
  type CredentialRef,
} from '../lib/credential-refs'
import { CredentialRefCandidateList } from './credential-ref-candidate-list'
import { useCredentialRefExpansion } from './credential-ref-candidates'
import { UnresolvedCredentialRefs } from './credential-ref-unresolved'
import { ScopePickerToolbar } from './scope-picker-toolbar'

type CredentialRefPickerProps = {
  value: CredentialRef[]
  onChange: (refs: CredentialRef[]) => void
} & Omit<ComponentProps<'div'>, 'onChange'>

export function CredentialRefPicker({
  value,
  onChange,
  ...props
}: CredentialRefPickerProps) {
  const { t } = useTranslation()
  // Composite control with three root states (loading / error / list); each is a
  // group named via aria-labelledby -> the FormLabel id, and each forwards the
  // FormControl-injected id / aria-describedby / aria-invalid (#1300).
  const labelId = props.id ? formLabelIdFor(props.id) : undefined
  const [search, setSearch] = useState('')
  const [onlySelected, setOnlySelected] = useState(false)
  const focus = useScopeSelectionFocus()
  const sitesQuery = useSites()
  const accountsQuery = useAccounts()
  const tokensQuery = useAllAccountTokens()

  const selectedKeys = useMemo(
    () => new Set(value.map(credentialRefKey)),
    [value]
  )

  const expansion = useCredentialRefExpansion(value)

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

  function toggleRef(ref: CredentialRef, checked: boolean) {
    const key = credentialRefKey(ref)
    const next = value.filter((item) => credentialRefKey(item) !== key)
    if (checked) next.push(ref)
    onChange(serializeCredentialRefs(next))
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
        role='group'
        aria-labelledby={labelId}
        {...props}
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
        role='group'
        aria-labelledby={labelId}
        {...props}
      >
        {t('settings.downstream.keys.credentials.loadFailed')}
      </div>
    )
  }

  return (
    <div
      data-testid='credential-ref-picker'
      className='space-y-2'
      role='group'
      aria-labelledby={labelId}
      {...props}
    >
      <ScopePickerToolbar
        selectedCount={selectedKeys.size}
        search={search}
        onSearchChange={setSearch}
        onlySelected={onlySelected}
        onOnlySelectedChange={setOnlySelected}
        onLocateSelected={() => {
          setSearch('')
          setOnlySelected(true)
          focus.focusSelected()
        }}
      />
      <div
        ref={focus.listRef}
        className='border-border max-h-56 overflow-y-auto rounded-md border'
      >
        <CredentialRefCandidateList
          sites={sites}
          accountsBySite={accountsBySite}
          tokensByAccount={tokensByAccount}
          value={value}
          selectedKeys={selectedKeys}
          search={search}
          onlySelected={onlySelected}
          expansion={expansion}
          toggleRef={toggleRef}
        />
        <UnresolvedCredentialRefs
          value={value}
          sites={sites}
          accounts={accountsQuery.data?.accounts ?? []}
          tokens={tokensQuery.data ?? []}
          onRemove={(ref) => toggleRef(ref, false)}
        />
      </div>
    </div>
  )
}
