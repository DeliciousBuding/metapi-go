import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { CredentialRefPicker } from '../credential-ref-picker'

const { mockGetSites, mockGetAccountsSnapshot, mockGetAccountTokens } =
  vi.hoisted(() => ({
    mockGetSites: vi.fn(),
    mockGetAccountsSnapshot: vi.fn(),
    mockGetAccountTokens: vi.fn(),
  }))

vi.mock('@/lib/api', () => ({
  api: {
    getSites: mockGetSites,
    getAccountsSnapshot: mockGetAccountsSnapshot,
    getAccountTokens: mockGetAccountTokens,
  },
}))

beforeEach(() => {
  mockGetSites.mockReset()
  mockGetAccountsSnapshot.mockReset()
  mockGetAccountTokens.mockReset()
  mockGetSites.mockResolvedValue([{ id: 1, name: 'Alpha' }])
  mockGetAccountsSnapshot.mockResolvedValue({
    generatedAt: '2026-08-29T00:00:00Z',
    accounts: [
      {
        id: 11,
        siteId: 1,
        username: 'alice',
        apiTokenMasked: 'sk-***abc',
      },
      { id: 12, siteId: 1, username: 'bob', apiTokenMasked: null },
    ],
    sites: [{ id: 1, name: 'Alpha' }],
  })
  mockGetAccountTokens.mockResolvedValue([
    { id: 101, accountId: 11, name: 'Token A', tokenMasked: 'sk-***a' },
  ])
})

afterEach(() => cleanup())

function renderPicker(
  value: Parameters<typeof CredentialRefPicker>[0]['value']
) {
  const onChange = vi.fn()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  })
  const result = render(
    (
      <QueryClientProvider client={queryClient}>
        <CredentialRefPicker value={value} onChange={onChange} />
      </QueryClientProvider>
    ) as ReactElement
  )
  return { ...result, onChange }
}

async function expandAlpha() {
  fireEvent.click(
    await screen.findByRole('button', { name: 'Toggle accounts of Alpha' })
  )
}

describe('CredentialRefPicker', () => {
  it('selects an account default API key and emits a canonical ref', async () => {
    const { onChange } = renderPicker([])
    await expandAlpha()

    fireEvent.click(await screen.findByRole('checkbox', { name: /alice/ }))

    expect(onChange).toHaveBeenCalledWith([
      { kind: 'default_api_key', siteId: 1, accountId: 11 },
    ])
  })

  it('disables the default-key checkbox when the account has no default key', async () => {
    renderPicker([])
    await expandAlpha()

    const checkbox = await screen.findByRole('checkbox', { name: /bob/ })
    expect(checkbox).toBeDisabled()
  })

  it('pre-fills account_token and default_api_key refs and expands the right branches', async () => {
    renderPicker([
      { kind: 'default_api_key', siteId: 1, accountId: 11 },
      { kind: 'account_token', siteId: 1, accountId: 11, tokenId: 101 },
    ])

    const picker = await screen.findByTestId('credential-ref-picker')
    const accountCheckbox = await within(picker).findByRole('checkbox', {
      name: /alice/,
    })
    const tokenCheckbox = within(picker).getByRole('checkbox', {
      name: /Token Token A/,
    })

    expect(accountCheckbox).toBeChecked()
    expect(tokenCheckbox).toBeChecked()
  })

  it('surfaces unresolved refs and lets the operator remove them', async () => {
    const { onChange } = renderPicker([
      {
        kind: 'account_token',
        siteId: 90,
        accountId: 91,
        tokenId: 92,
      },
    ])

    const remove = await screen.findByRole('button', {
      name: 'Remove unresolved reference',
    })
    fireEvent.click(remove)

    expect(onChange).toHaveBeenCalledWith([])
  })

  it('keeps the expander and picker controls keyboard-focusable', async () => {
    renderPicker([])
    await expandAlpha()

    const expander = screen.getByRole('button', {
      name: 'Toggle accounts of Alpha',
    })
    expander.focus()
    expect(document.activeElement).toBe(expander)
    expect(
      await screen.findByRole('checkbox', { name: /alice/ })
    ).toHaveAttribute('type', 'checkbox')
  })
})
