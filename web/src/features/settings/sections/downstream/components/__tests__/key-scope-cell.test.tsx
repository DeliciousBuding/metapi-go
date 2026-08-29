import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { KeyScopeCell, type ScopeNameMaps } from '../key-scope-cell'

afterEach(() => cleanup())

const names: ScopeNameMaps = {
  sites: new Map([
    [1, 'Alpha'],
    [2, 'Beta'],
  ]),
  accounts: new Map([
    [11, 'alice'],
    [12, 'bob'],
  ]),
  tokens: new Map([[101, 'Token A']]),
}

function renderCell(item: Parameters<typeof KeyScopeCell>[0]['item']) {
  return render((<KeyScopeCell item={item} names={names} />) as ReactElement)
}

describe('KeyScopeCell', () => {
  it('renders unrestricted when no policy dimensions are configured', () => {
    renderCell({})
    expect(screen.getByText('Unrestricted')).toBeInTheDocument()
  })

  it('resolves site/account/token refs to human-readable names', () => {
    renderCell({
      allowedCredentialRefs: JSON.stringify([
        { kind: 'account_token', siteId: 1, accountId: 11, tokenId: 101 },
        { kind: 'default_api_key', siteId: 2, accountId: 12 },
      ]),
    })
    expect(screen.getByText(/Alpha \/ alice \/ Token A/)).toBeInTheDocument()
    expect(screen.getByText(/Beta \/ bob \/ default key/)).toBeInTheDocument()
  })

  it('shows allow and exclude dimensions separately', () => {
    renderCell({
      allowedSiteIds: '[1]',
      excludedCredentialRefs: JSON.stringify([
        { kind: 'default_api_key', siteId: 2, accountId: 12 },
      ]),
    })
    expect(screen.getByText('Allowed sites:')).toBeInTheDocument()
    expect(screen.getByText('Excluded credentials:')).toBeInTheDocument()
  })

  it('honestly renders missing mappings as unresolved IDs', () => {
    renderCell({
      allowedCredentialRefs: JSON.stringify([
        { kind: 'account_token', siteId: 7, accountId: 8, tokenId: 9 },
      ]),
    })
    expect(
      screen.getByText(
        /ID 7 \(unresolved\) \/ ID 8 \(unresolved\) \/ ID 9 \(unresolved\)/
      )
    ).toBeInTheDocument()
  })
})
