// Regression guard for the credential-mode UI used by the account-creation
// browser smoke. The real-browser gate owns renderer-loop detection; this unit
// test keeps the three modes and password field transition cheap to diagnose.
import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import type { Site } from '../../types'
import { AccountFormDialog } from '../account-form-dialog'

vi.mock('../../api', () => ({
  useCreateAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useLoginAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useVerifyAccountToken: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

afterEach(() => cleanup())

const sites: Site[] = [
  {
    id: 1,
    name: 'Browser Smoke Site',
    url: 'https://example.invalid',
    platform: 'new-api',
    status: 'active',
  },
]

describe('AccountFormDialog credential modes', () => {
  it('renders all modes and switches to password fields', async () => {
    render(
      <AccountFormDialog
        open
        onOpenChange={() => {}}
        mode='create'
        account={null}
        sites={sites}
      />
    )

    await waitFor(() => {
      expect(screen.getAllByRole('tab')).toHaveLength(3)
    })

    const passwordTab = screen.getByRole('tab', { name: /密码|Password/i })
    fireEvent.click(passwordTab)

    await waitFor(() => {
      expect(screen.getByLabelText(/密码|Password/i)).toBeInTheDocument()
    })
  })
})

describe('AccountFormDialog dirty-close guard', () => {
  it('opens the discard confirm when Cancel is clicked with unsaved edits', async () => {
    const onOpenChange = vi.fn()
    render(
      <AccountFormDialog
        open
        onOpenChange={onOpenChange}
        mode='create'
        account={null}
        sites={sites}
      />
    )

    const accessTokenField = await screen.findByLabelText(
      'Access Token / Cookie'
    )
    fireEvent.change(accessTokenField, { target: { value: 'dirty-token' } })

    // The explicit Cancel button must route through the same dirty-close
    // guard as Esc/X — never silently discard the input (issue #889).
    fireEvent.click(screen.getByRole('button', { name: /取消|Cancel/i }))

    await waitFor(() => {
      expect(
        screen.getByText(/放弃未保存的更改？|Discard unsaved changes\?/)
      ).toBeInTheDocument()
    })
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })
})
