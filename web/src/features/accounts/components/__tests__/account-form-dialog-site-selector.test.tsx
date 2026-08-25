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

import { accountSchema, type Site } from '../../types'
import { AccountFormDialog } from '../account-form-dialog'

const mutations = vi.hoisted(() => ({
  create: { mutateAsync: vi.fn(), isPending: false },
  login: { mutateAsync: vi.fn(), isPending: false },
  update: { mutateAsync: vi.fn(), isPending: false },
  verify: { mutateAsync: vi.fn() },
}))
const showAccountCreatedToast = vi.hoisted(() => vi.fn())

vi.mock('../../api', () => ({
  resolveCreatedAccountId: (result: { id?: number } | undefined) => result?.id,
  useCreateAccount: () => mutations.create,
  useLoginAccount: () => mutations.login,
  useUpdateAccount: () => mutations.update,
  useVerifyAccountToken: () => mutations.verify,
}))

vi.mock('../account-created-toast', () => ({ showAccountCreatedToast }))

const sites: Site[] = [
  ...Array.from({ length: 32 }, (_, index) => ({
    id: index + 1,
    name: `Fixture Site ${String(index + 1).padStart(2, '0')}`,
    url: `https://fixture-${index + 1}.example`,
    platform: index % 2 === 0 ? 'openai' : 'one-api',
    status: 'active',
  })),
  {
    id: 101,
    name: 'Aurora Gateway',
    url: 'https://aurora.example/api',
    platform: 'new-api',
    status: 'active',
  },
  {
    id: 102,
    name: 'Lunar Relay',
    url: 'https://lunar.example/v1',
    platform: 'done-hub',
    status: 'active',
  },
  {
    id: 103,
    name: 'Nebula Hub',
    url: 'https://nebula.example',
    platform: 'sub2api',
    status: 'active',
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

  Element.prototype.scrollIntoView = vi.fn()
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

function renderCreate(initialSiteId?: number) {
  return render(
    <AccountFormDialog
      open
      onOpenChange={vi.fn()}
      mode='create'
      sites={sites}
      initialSiteId={initialSiteId}
    />
  )
}

async function openSiteSearch() {
  const trigger = await screen.findByRole('combobox', { name: 'Site' })
  fireEvent.click(trigger)
  const input = await screen.findByPlaceholderText('Search sites…')
  return { trigger, input }
}

describe('AccountFormDialog searchable site selector', () => {
  it.each([
    ['name', 'Aurora Gateway', /Aurora Gateway/i],
    ['URL', 'lunar.example', /Lunar Relay/i],
    ['platform', 'sub2api', /Nebula Hub/i],
  ])('filters 30+ local sites by %s', async (_field, query, expectedName) => {
    renderCreate()

    const { input } = await openSiteSearch()
    fireEvent.change(input, { target: { value: query } })

    expect(
      await screen.findByRole('option', { name: expectedName })
    ).toBeVisible()
    expect(
      screen.queryByRole('option', { name: /Fixture Site 01/i })
    ).not.toBeInTheDocument()
  })

  it('selects by keyboard and submits a numeric siteId', async () => {
    mutations.create.mutateAsync.mockResolvedValue({ id: 77 })
    renderCreate()

    const { input } = await openSiteSearch()
    await waitFor(() => expect(input).toHaveFocus())

    fireEvent.change(input, { target: { value: 'lunar.example' } })
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => {
      expect(screen.getByRole('combobox', { name: 'Site' })).toHaveTextContent(
        'Lunar Relay'
      )
    })

    fireEvent.change(screen.getByLabelText('Access Token / Cookie'), {
      target: { value: 'session-token' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Add account' }))

    await waitFor(() => {
      expect(mutations.create.mutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({ siteId: 102 })
      )
    })
    const payload = mutations.create.mutateAsync.mock.calls[0]?.[0]
    expect(typeof payload.siteId).toBe('number')
    expect(showAccountCreatedToast).toHaveBeenCalledWith(77, 102)
  })

  it('preserves the initialSiteId deep-link selection', async () => {
    renderCreate(103)

    expect(
      await screen.findByRole('combobox', { name: 'Site' })
    ).toHaveTextContent('Nebula Hub')
  })

  it('keeps the edit-mode site until the operator selects another option', async () => {
    const account = accountSchema.parse({
      id: 900,
      siteId: 102,
      credentialMode: 'session',
      username: 'Existing account',
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

    const { trigger, input } = await openSiteSearch()
    expect(trigger).toHaveTextContent('Lunar Relay')

    fireEvent.change(input, { target: { value: 'Aurora Gateway' } })
    expect(
      await screen.findByRole('option', { name: /Aurora Gateway/i })
    ).toBeVisible()

    fireEvent.click(trigger)
    await waitFor(() =>
      expect(trigger).toHaveAttribute('aria-expanded', 'false')
    )
    expect(trigger).toHaveTextContent('Lunar Relay')
  })
})
