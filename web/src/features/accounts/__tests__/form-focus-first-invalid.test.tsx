// Focus-first-invalid verification (audit P2 #5 closeout): the design
// system's <Form> ships FormValidationFocus, which after a failed submit
// scrolls to and focuses the first aria-invalid control. The audit listed
// account-form-dialog as still missing this behavior; this test proves the
// mechanism works end-to-end on the account dialog (Sheet drawer, Select
// first field, footer submit button bound through the `form` attribute).

import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import { AccountFormDialog } from '../components/account-form-dialog'
import type { Site } from '../types'

const { mockToastError } = vi.hoisted(() => ({ mockToastError: vi.fn() }))

vi.mock('@/lib/toast', () => ({
  toast: {
    error: mockToastError,
    success: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
    message: vi.fn(),
    loading: vi.fn(),
  },
}))

vi.mock('../api', () => ({
  useCreateAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useLoginAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useVerifyAccountToken: () => ({ mutateAsync: vi.fn(), isPending: false }),
  resolveCreatedAccountId: () => 1,
}))

vi.mock('../components/account-created-toast', () => ({
  showAccountCreatedToast: vi.fn(),
  showAccountLoginToast: vi.fn(),
}))

const probeSite = {
  id: 7,
  siteId: 0,
  name: 'Probe site',
  url: 'https://probe.example.com',
  platform: 'new-api',
} as unknown as Site

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
  // jsdom does not implement scrollIntoView; FormValidationFocus calls it
  // before focusing the invalid control.
  Element.prototype.scrollIntoView = vi.fn()
})

beforeEach(() => {
  mockToastError.mockReset()
})

afterEach(() => cleanup())

describe('AccountFormDialog focus-first-invalid', () => {
  it('focuses the first invalid field when submitting an empty form', async () => {
    render(
      <AccountFormDialog
        open
        onOpenChange={() => {}}
        mode='create'
        sites={[probeSite]}
      />
    )

    const submitButton = await screen.findByRole('button', {
      name: 'Add account',
    })
    await waitFor(() => {
      expect(submitButton).toBeEnabled()
    })

    fireEvent.click(submitButton)

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledTimes(1)
    })

    // The site Select is the first form field; its trigger must be marked
    // invalid and receive focus so the operator lands on the error.
    await waitFor(() => {
      const invalidControl = document.querySelector<HTMLElement>(
        '#account-form [aria-invalid="true"]'
      )
      expect(invalidControl).not.toBeNull()
      expect(document.activeElement).toBe(invalidControl)
    })
  })
})
