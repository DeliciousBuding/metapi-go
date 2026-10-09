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
import type { SiteInitializationPreset } from '@/lib/api/sites'

import { accountSchema } from '../types'
import { AccountFormDialog } from './account-form-dialog'

const mutations = vi.hoisted(() => ({
  create: { mutateAsync: vi.fn(), isPending: false },
  login: { mutateAsync: vi.fn(), isPending: false },
  update: { mutateAsync: vi.fn(), isPending: false },
  verify: { mutateAsync: vi.fn() },
}))
const presetState = vi.hoisted(() => ({
  data: [] as SiteInitializationPreset[],
  isSuccess: true,
}))
const showAccountCreatedToast = vi.hoisted(() => vi.fn())
const showAccountLoginToast = vi.hoisted(() => vi.fn())
const toastMocks = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}))

vi.mock('@/features/sites', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/sites')>()),
  useSiteInitializationPresets: () => presetState,
}))

vi.mock('@/lib/toast', () => ({ toast: toastMocks }))

vi.mock('../api', () => ({
  resolveCreatedAccountId: (result: { id?: number } | undefined) => result?.id,
  useCreateAccount: () => mutations.create,
  useLoginAccount: () => mutations.login,
  useUpdateAccount: () => mutations.update,
  useVerifyAccountToken: () => mutations.verify,
}))

vi.mock('./account-created-toast', () => ({
  showAccountCreatedToast,
  showAccountLoginToast,
}))

const sites = [
  {
    id: 7,
    name: 'Primary site',
    url: 'https://primary.example',
    platform: 'new-api',
    status: 'active' as const,
  },
]

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
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
  presetState.data = []
  presetState.isSuccess = true
  mutations.create.mutateAsync.mockReset()
  mutations.login.mutateAsync.mockReset()
  mutations.update.mutateAsync.mockReset()
  mutations.verify.mutateAsync.mockReset()
  showAccountCreatedToast.mockReset()
  showAccountLoginToast.mockReset()
  toastMocks.success.mockReset()
  toastMocks.warning.mockReset()
  toastMocks.error.mockReset()
  toastMocks.info.mockReset()
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
      expect(screen.getByLabelText('Access Token / Cookie')).toBeInTheDocument()
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
      expect(showAccountCreatedToast).toHaveBeenCalledWith(42, 7, {
        tokenCount: undefined,
        tokenSyncStatus: undefined,
        tokenSyncMessage: undefined,
      })
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

describe('AccountFormDialog token sync truthfulness', () => {
  it('forwards the create sync report to the guided route toast', async () => {
    mutations.create.mutateAsync.mockResolvedValue({
      id: 42,
      tokenCount: 3,
      tokenSyncStatus: 'synced',
      tokenSyncMessage: 'synced 3 tokens',
    })
    renderAccountForm()

    fireEvent.change(await screen.findByLabelText('Access Token / Cookie'), {
      target: { value: 'session-token' },
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Add account' }))

    await waitFor(() => {
      expect(showAccountCreatedToast).toHaveBeenCalledWith(42, 7, {
        tokenCount: 3,
        tokenSyncStatus: 'synced',
        tokenSyncMessage: 'synced 3 tokens',
      })
    })
  })

  it('downgrades a failed login sync to a partial-initialization warning', async () => {
    mutations.login.mutateAsync.mockResolvedValue({
      success: true,
      account: { id: 9 },
      tokenCount: 0,
      tokenSyncStatus: 'failed',
      tokenSyncMessage:
        'partial initialization: token sync failed: upstream 500',
    })
    renderAccountForm()

    fireEvent.click(await screen.findByRole('tab', { name: 'Password' }))
    fireEvent.change(await screen.findByLabelText('Username'), {
      target: { value: 'site-user' },
    })
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'site-pass' },
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Add account' }))

    await waitFor(() => {
      expect(showAccountLoginToast).toHaveBeenCalledWith(9, 7, undefined, {
        tokenCount: 0,
        tokenSyncStatus: 'failed',
        tokenSyncMessage:
          'partial initialization: token sync failed: upstream 500',
      })
    })
    expect(showAccountCreatedToast).not.toHaveBeenCalled()
  })

  it('keeps the plain success toast when the login sync is healthy', async () => {
    mutations.login.mutateAsync.mockResolvedValue({
      success: true,
      account: { id: 9 },
      tokenCount: 2,
      tokenSyncStatus: 'synced',
      tokenSyncMessage: 'synced 2 tokens',
    })
    renderAccountForm()

    fireEvent.click(await screen.findByRole('tab', { name: 'Password' }))
    fireEvent.change(await screen.findByLabelText('Username'), {
      target: { value: 'site-user' },
    })
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'site-pass' },
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Add account' }))

    await waitFor(() => {
      expect(showAccountLoginToast).toHaveBeenCalledWith(9, 7, undefined, {
        tokenCount: 2,
        tokenSyncStatus: 'synced',
        tokenSyncMessage: 'synced 2 tokens',
      })
    })
    expect(toastMocks.success).not.toHaveBeenCalled()
    expect(toastMocks.warning).not.toHaveBeenCalled()
  })
})

const codingSite = {
  id: 8,
  name: 'Coding plan',
  url: 'https://CODING.EXAMPLE.INVALID:443/api/coding/?from=console#anchor',
  platform: 'openai',
  status: 'active',
}
const standardSite = {
  id: 9,
  name: 'Standard API',
  url: 'https://standard.example.invalid/v1',
  platform: 'openai',
  status: 'active',
}
const codingPreset: SiteInitializationPreset = {
  id: 'fixture-coding',
  label: 'Fixture Coding',
  providerLabel: 'Fixture',
  platform: 'openai',
  defaultUrl: 'https://coding.example.invalid/api/coding/',
  recommendedSkipModelFetch: true,
  recommendedModels: ['coding-model'],
  docsUrl: '',
}
const onboardingSites = [...sites, codingSite, standardSite]
function onboardingForm(initialSiteId = 8) {
  return (
    <AccountFormDialog
      open
      onOpenChange={vi.fn()}
      mode='create'
      sites={onboardingSites}
      initialSiteId={initialSiteId}
    />
  )
}
async function selectOnboardingSite(name: string) {
  fireEvent.click(screen.getByRole('combobox', { name: 'Site' }))
  const search = await screen.findByPlaceholderText('Search sites…')
  fireEvent.change(search, { target: { value: name } })
  fireEvent.click(await screen.findByRole('option', { name: new RegExp(name) }))
  await waitFor(() =>
    expect(screen.getByRole('combobox', { name: 'Site' })).toHaveTextContent(
      name
    )
  )
}
function submitOnboardingForm() {
  const form = document.querySelector('form')
  if (!form) throw new Error('Missing account form')
  fireEvent.submit(form)
}
function skipDiscoverySwitch() {
  return screen.getByRole('switch', { name: /Skip model fetch/i })
}

describe('AccountFormDialog preset onboarding', () => {
  it('defaults matched CodingPlan API keys to skip discovery and sends that value without pretending to verify', async () => {
    presetState.data = [codingPreset]
    render(onboardingForm())
    await waitFor(() => expect(skipDiscoverySwitch()).toBeChecked())
    expect(screen.getByRole('tab', { name: 'API Key' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(
      screen.queryByRole('tab', { name: 'Password' })
    ).not.toBeInTheDocument()
    fireEvent.change(screen.getByRole('textbox', { name: 'API Key' }), {
      target: { value: 'fixture-coding-key' },
    })
    submitOnboardingForm()
    await waitFor(() =>
      expect(mutations.create.mutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          siteId: 8,
          credentialMode: 'apikey',
          skipModelFetch: true,
          accessTokens: ['fixture-coding-key'],
        })
      )
    )
    expect(mutations.verify.mutateAsync).not.toHaveBeenCalled()
    expect(mutations.login.mutateAsync).not.toHaveBeenCalled()
  })

  it('uses recommendations on each untouched site selection, without carrying a CodingPlan default to another URL', async () => {
    presetState.data = [codingPreset]
    render(onboardingForm())
    await waitFor(() => expect(skipDiscoverySwitch()).toBeChecked())
    await selectOnboardingSite('Standard API')
    await waitFor(() => expect(skipDiscoverySwitch()).not.toBeChecked())
    await selectOnboardingSite('Coding plan')
    await waitFor(() => expect(skipDiscoverySwitch()).toBeChecked())
    fireEvent.click(skipDiscoverySwitch())
    await selectOnboardingSite('Standard API')
    await selectOnboardingSite('Coding plan')
    expect(skipDiscoverySwitch()).not.toBeChecked()
  })

  it('does not overwrite an explicit toggle back to false when the preset arrives late', async () => {
    presetState.isSuccess = false
    const { rerender } = render(onboardingForm())
    await waitFor(() => expect(skipDiscoverySwitch()).not.toBeChecked())
    fireEvent.click(skipDiscoverySwitch())
    fireEvent.click(skipDiscoverySwitch())
    presetState.data = [codingPreset]
    presetState.isSuccess = true
    rerender(onboardingForm())
    await waitFor(() => expect(skipDiscoverySwitch()).not.toBeChecked())
    fireEvent.change(screen.getByRole('textbox', { name: 'API Key' }), {
      target: { value: 'fixture-key' },
    })
    submitOnboardingForm()
    await waitFor(() =>
      expect(mutations.create.mutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({ skipModelFetch: false })
      )
    )
  })

  it('applies a late preset to an untouched field, and resets user choices on a fresh open', async () => {
    presetState.isSuccess = false
    const { rerender } = render(onboardingForm())
    await waitFor(() => expect(skipDiscoverySwitch()).not.toBeChecked())
    presetState.data = [codingPreset]
    presetState.isSuccess = true
    rerender(onboardingForm())
    await waitFor(() => expect(skipDiscoverySwitch()).toBeChecked())
    fireEvent.click(skipDiscoverySwitch())
    rerender(
      <AccountFormDialog
        open={false}
        onOpenChange={vi.fn()}
        mode='create'
        sites={onboardingSites}
        initialSiteId={8}
      />
    )
    rerender(onboardingForm())
    await waitFor(() => expect(skipDiscoverySwitch()).toBeChecked())
  })

  it('keeps historical API account editing unchanged', async () => {
    presetState.data = [codingPreset]
    const account = accountSchema.parse({
      id: 90,
      siteId: 8,
      credentialMode: 'apikey',
      status: 'active',
      username: 'existing',
    })
    render(
      <AccountFormDialog
        open
        onOpenChange={vi.fn()}
        mode='edit'
        account={account}
        sites={onboardingSites}
      />
    )
    await waitFor(() => expect(skipDiscoverySwitch()).not.toBeChecked())
    expect(screen.getByRole('tab', { name: 'Password' })).toBeVisible()
    submitOnboardingForm()
    await waitFor(() =>
      expect(mutations.update.mutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({
          id: 90,
          payload: expect.objectContaining({ skipModelFetch: false }),
        })
      )
    )
  })

  it('preserves a typed password draft when switching to an API site', async () => {
    render(onboardingForm(7))
    fireEvent.click(await screen.findByRole('tab', { name: 'Password' }))
    fireEvent.change(screen.getByLabelText('Username'), {
      target: { value: 'draft-user' },
    })
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'draft-password' },
    })
    await selectOnboardingSite('Coding plan')
    expect(screen.getByRole('tab', { name: 'Password' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(screen.getByLabelText('Username')).toHaveValue('draft-user')
    expect(screen.getByLabelText('Password')).toHaveValue('draft-password')
    submitOnboardingForm()
    await waitFor(() =>
      expect(mutations.login.mutateAsync).toHaveBeenCalledWith({
        siteId: 8,
        username: 'draft-user',
        password: 'draft-password',
      })
    )
  })

  it('replaces an empty password mode with API key mode on an API site', async () => {
    render(onboardingForm(7))
    fireEvent.click(await screen.findByRole('tab', { name: 'Password' }))
    await selectOnboardingSite('Standard API')
    await waitFor(() =>
      expect(
        screen.queryByRole('tab', { name: 'Password' })
      ).not.toBeInTheDocument()
    )
    expect(screen.getByRole('tab', { name: 'API Key' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })
})
