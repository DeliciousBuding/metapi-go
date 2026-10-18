// Behavior test for the shared runtime-settings save path (#1034).
//
// Sensitive runtime fields (egress proxy, notify endpoints, admin IP ACL)
// make PUT /api/settings/runtime answer `403 reauthRequired`. The shared hook
// must open the master-token dialog and replay the same payload with
// X-Admin-Confirm-Token, while ordinary saves and non-reauth failures keep
// their existing behaviour (no dialog, section toast untouched).

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
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

import { ReauthProvider } from '@/components/common/reauth-provider'
import i18n from '@/i18n/config'

import { useUpdateRuntimeSettings } from '../runtime-settings'

const { mockUpdateRuntimeSettings } = vi.hoisted(() => ({
  mockUpdateRuntimeSettings: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    updateRuntimeSettings: mockUpdateRuntimeSettings,
  },
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

beforeEach(() => {
  mockUpdateRuntimeSettings.mockReset()
})

afterEach(() => cleanup())

const text = (key: string) => String(i18n.t(key))

function reauthError() {
  return { response: { status: 403, data: { reauthRequired: true } } }
}

const SENSITIVE_PAYLOAD = { systemProxyUrl: 'http://proxy.internal:3128' }

function Harness({
  onSuccess,
  onError,
}: {
  onSuccess: () => void
  onError: () => void
}) {
  const updateMutation = useUpdateRuntimeSettings()
  return (
    <button
      type='button'
      onClick={() =>
        updateMutation.mutate(SENSITIVE_PAYLOAD, { onSuccess, onError })
      }
    >
      Save
    </button>
  )
}

function renderHarness() {
  const onSuccess = vi.fn()
  const onError = vi.fn()
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ReauthProvider>
        <Harness onSuccess={onSuccess} onError={onError} />
      </ReauthProvider>
    </QueryClientProvider>
  )
  return { onSuccess, onError }
}

describe('useUpdateRuntimeSettings — master-token replay', () => {
  it('prompts once and replays the same payload with the token', async () => {
    mockUpdateRuntimeSettings
      .mockRejectedValueOnce(reauthError())
      .mockResolvedValueOnce({ success: true })
    const { onSuccess, onError } = renderHarness()

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(mockUpdateRuntimeSettings).toHaveBeenCalledTimes(1)
    )
    expect(mockUpdateRuntimeSettings).toHaveBeenCalledWith(SENSITIVE_PAYLOAD)
    // The section's generic "save failed" toast must not fire for a reauth
    // demand; the dialog owns the next step.
    expect(onError).not.toHaveBeenCalled()

    const input = await screen.findByLabelText(text('reauth.tokenLabel'))
    fireEvent.change(input, { target: { value: 'master-token' } })
    fireEvent.click(screen.getByRole('button', { name: text('reauth.submit') }))

    await waitFor(() =>
      expect(mockUpdateRuntimeSettings).toHaveBeenCalledTimes(2)
    )
    expect(mockUpdateRuntimeSettings).toHaveBeenNthCalledWith(
      2,
      SENSITIVE_PAYLOAD,
      'master-token'
    )
    await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
    expect(onError).not.toHaveBeenCalled()
  })

  it('keeps the dialog open when the replayed save is rejected again', async () => {
    mockUpdateRuntimeSettings
      .mockRejectedValueOnce(reauthError())
      .mockRejectedValueOnce(reauthError())
    const { onError } = renderHarness()

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await screen.findByLabelText(text('reauth.tokenLabel'))
    fireEvent.change(screen.getByLabelText(text('reauth.tokenLabel')), {
      target: { value: 'wrong-token' },
    })
    fireEvent.click(screen.getByRole('button', { name: text('reauth.submit') }))

    expect(await screen.findByText(text('reauth.errorInvalid'))).toBeVisible()
    expect(screen.getByLabelText(text('reauth.tokenLabel'))).toBeVisible()
    expect(onError).not.toHaveBeenCalled()
  })

  it('does not prompt for a non-reauth failure and keeps the section toast', async () => {
    mockUpdateRuntimeSettings.mockRejectedValueOnce({
      response: { status: 400, data: { error: 'invalid proxy url' } },
    })
    const { onSuccess, onError } = renderHarness()

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(onError).toHaveBeenCalledTimes(1))
    expect(onSuccess).not.toHaveBeenCalled()
    expect(
      screen.queryByLabelText(text('reauth.tokenLabel'))
    ).not.toBeInTheDocument()
  })

  it('does not prompt for an ordinary successful save', async () => {
    mockUpdateRuntimeSettings.mockResolvedValueOnce({ success: true })
    const { onSuccess, onError } = renderHarness()

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
    expect(onError).not.toHaveBeenCalled()
    expect(
      screen.queryByLabelText(text('reauth.tokenLabel'))
    ).not.toBeInTheDocument()
  })
})
