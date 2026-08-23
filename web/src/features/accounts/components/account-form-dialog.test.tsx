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

import i18n from '@/i18n/config'

import { accountSchema } from '../types'
import { AccountFormDialog } from './account-form-dialog'

const mutations = vi.hoisted(() => ({
  create: { mutateAsync: vi.fn(), isPending: false },
  login: { mutateAsync: vi.fn(), isPending: false },
  update: { mutateAsync: vi.fn(), isPending: false },
  verify: { mutateAsync: vi.fn() },
}))
const showAccountCreatedToast = vi.hoisted(() => vi.fn())

vi.mock('../api', () => ({
  resolveCreatedAccountId: (result: { id?: number } | undefined) => result?.id,
  useCreateAccount: () => mutations.create,
  useLoginAccount: () => mutations.login,
  useUpdateAccount: () => mutations.update,
  useVerifyAccountToken: () => mutations.verify,
}))

vi.mock('./account-created-toast', () => ({ showAccountCreatedToast }))

const sites = [
  {
    id: 7,
    name: 'Primary site',
    url: 'https://primary.example',
    platform: 'openai',
    status: 'active' as const,
  },
]

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

  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {
    writable: true,
    value: ResizeObserverStub,
  })
})

beforeEach(async () => {
  await i18n.changeLanguage('en')
  mutations.create.mutateAsync.mockReset()
  mutations.login.mutateAsync.mockReset()
  mutations.update.mutateAsync.mockReset()
  mutations.verify.mutateAsync.mockReset()
  showAccountCreatedToast.mockReset()
})

afterEach(() => cleanup())

function renderAccountForm() {
  return render(
    <AccountFormDialog
      open
      onOpenChange={vi.fn()}
      mode='create'
      sites={sites}
      initialSiteId={7}
    />
  )
}

describe('AccountFormDialog credential verification', () => {
  it('preselects the site supplied by the accounts deep link', async () => {
    renderAccountForm()

    const siteSelect = await screen.findByRole('combobox', { name: 'Site' })

    expect(siteSelect).toHaveTextContent('Primary site')
  })

  it('clears a successful verification when the credential changes', async () => {
    mutations.verify.mutateAsync.mockResolvedValue({
      tokenType: 'session',
      modelCount: 4,
    })
    renderAccountForm()

    const credentialInput = await screen.findByLabelText(
      'Access Token / Cookie'
    )
    fireEvent.change(credentialInput, {
      target: { value: 'session-token' },
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Verify' }))

    await screen.findByText(/Verified/)
    fireEvent.change(credentialInput, {
      target: { value: 'replacement-token' },
    })

    await waitFor(() => {
      expect(screen.queryByText(/Verified/)).not.toBeInTheDocument()
    })
  })

  it('does not expose inline verification in password mode', async () => {
    renderAccountForm()

    fireEvent.click(await screen.findByRole('tab', { name: 'Password' }))

    await waitFor(() => {
      expect(
        screen.queryByRole('button', { name: 'Verify' })
      ).not.toBeInTheDocument()
    })
    expect(mutations.verify.mutateAsync).not.toHaveBeenCalled()
  })

  it('ignores a verification result after the credential changes', async () => {
    let resolveVerification: ((value: unknown) => void) | undefined
    mutations.verify.mutateAsync.mockReturnValue(
      new Promise((resolve) => {
        resolveVerification = resolve
      })
    )
    renderAccountForm()

    const credentialInput = await screen.findByLabelText(
      'Access Token / Cookie'
    )
    fireEvent.change(credentialInput, { target: { value: 'first-token' } })
    fireEvent.click(await screen.findByRole('button', { name: 'Verify' }))
    fireEvent.change(credentialInput, {
      target: { value: 'replacement-token' },
    })
    resolveVerification?.({ tokenType: 'session', modelCount: 9 })

    await waitFor(() => {
      expect(screen.queryByText(/Verified/)).not.toBeInTheDocument()
    })
  })
})

describe('AccountFormDialog deep-link credential mode hint', () => {
  function renderWithMode(initialCredentialMode: 'apikey' | undefined) {
    return render(
      <AccountFormDialog
        open
        onOpenChange={vi.fn()}
        mode='create'
        sites={sites}
        initialSiteId={7}
        initialCredentialMode={initialCredentialMode}
      />
    )
  }

  it('opens in apikey mode when the deep link passed segment=apikey', async () => {
    renderWithMode('apikey')

    // The apikey tab is selected: the session-token field is absent and the
    // API token field is present.
    await waitFor(() => {
      expect(
        screen.queryByLabelText('Access Token / Cookie')
      ).not.toBeInTheDocument()
    })
    expect(await screen.findByLabelText('API Key')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'API Key' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(screen.getByRole('tab', { name: 'Session' })).toHaveAttribute(
      'aria-selected',
      'false'
    )
  })

  it('keeps the session default when no segment hint is present', async () => {
    renderWithMode(undefined)

    await waitFor(() => {
      expect(
        screen.getByLabelText('Access Token / Cookie')
      ).toBeInTheDocument()
    })
    expect(screen.getByRole('tab', { name: 'Session' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })
})

describe('AccountFormDialog submission contracts', () => {
  it('hands the top-level created account id to the guided route toast', async () => {
    mutations.create.mutateAsync.mockResolvedValue({ id: 42 })
    renderAccountForm()

    fireEvent.change(await screen.findByLabelText('Access Token / Cookie'), {
      target: { value: 'session-token' },
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Add account' }))

    await waitFor(() => {
      expect(showAccountCreatedToast).toHaveBeenCalledWith(42, 7)
    })
  })

  it('updates non-secret fields without replacing a redacted credential', async () => {
    mutations.update.mutateAsync.mockResolvedValue({ success: true })
    const account = accountSchema.parse({
      id: 12,
      siteId: 7,
      credentialMode: 'session',
      username: 'Original name',
      status: 'active',
    })

    render(
      <AccountFormDialog
        open
        onOpenChange={vi.fn()}
        mode='edit'
        account={account}
        sites={sites}
      />
    )

    fireEvent.change(
      await screen.findByLabelText('Connection name (optional)'),
      { target: { value: 'Updated name' } }
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(mutations.update.mutateAsync).toHaveBeenCalledWith({
        id: 12,
        payload: expect.objectContaining({
          username: 'Updated name',
          accessToken: undefined,
        }),
      })
    })
  })
})
