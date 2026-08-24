// Fake-timer lifecycle tests for the OAuth start-dialog polling loop.
//
// Complements oauth-start-dialog-session.test.tsx (real timers, interactions):
// this file drives the clock to verify the bounded polling contract end to
// end through the dialog — success within the budget closes the dialog with
// feedback, budget exhaustion renders an honest "still waiting" state (never
// a fake success), a polled backend error stays visible, and closing the
// dialog while pending stops all further session checks.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import {
  OAUTH_SESSION_POLL_INTERVAL_MS,
  OAUTH_SESSION_POLL_MAX_ATTEMPTS,
} from '../../lib/oauth-session-polling'
import type { OAuthStartInstructions } from '../../types'
import { OAuthStartDialog } from '../oauth-start-dialog'

// ---------------------------------------------------------------------------
// Shared jsdom stubs (Base UI dropdown positioning needs both)
// ---------------------------------------------------------------------------

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

  // jsdom prints "Not implemented: navigation" when window.open is called.
  vi.stubGlobal('open', vi.fn())
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.clearAllMocks()
})

afterAll(() => {
  vi.restoreAllMocks()
})

// ---------------------------------------------------------------------------
// Mock fixtures
// ---------------------------------------------------------------------------

const TEST_STATE = 'test-session-state'

const TEST_INSTRUCTIONS: OAuthStartInstructions = {
  redirectUri: 'http://localhost:8080/callback',
  callbackPort: 8080,
  callbackPath: '/callback',
  manualCallbackDelayMs: 5000,
}

const TEST_START_RESULT = {
  provider: 'openai',
  state: TEST_STATE,
  authorizationUrl: 'https://provider.example.com/auth',
  instructions: TEST_INSTRUCTIONS,
}

const mockState = vi.hoisted(() => ({
  startMutate: vi.fn(),
  getSession: vi.fn(),
  callbackMutate: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockState.toastSuccess,
    error: mockState.toastError,
  },
}))

vi.mock('../../api', () => ({
  useOAuthProviders: () => ({
    data: [
      {
        provider: 'openai',
        label: 'OpenAI',
        platform: 'openai',
        enabled: true,
        loginType: 'oauth',
        requiresProjectId: false,
        supportsDirectAccountRouting: true,
        supportsCloudValidation: true,
        supportsNativeProxy: true,
      },
    ],
    isLoading: false,
    isError: false,
  }),
  useStartOAuth: () => ({
    mutateAsync: mockState.startMutate,
    isPending: false,
  }),
  useSubmitOAuthManualCallback: () => ({
    mutateAsync: mockState.callbackMutate,
    isPending: false,
  }),
}))

// The start dialog uses the REAL `useOAuthSessionPolling` hook, which polls
// `api.getOAuthSession` — stub the transport layer only.
vi.mock('@/lib/api', () => ({
  api: { getOAuthSession: mockState.getSession },
}))

function pendingSession() {
  return { provider: 'openai', state: TEST_STATE, status: 'pending' as const }
}

function successSession() {
  return { provider: 'openai', state: TEST_STATE, status: 'success' as const }
}

function renderDialog(onOpenChange = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
  return render(<OAuthStartDialog open onOpenChange={onOpenChange} />, {
    wrapper: Wrapper,
  })
}

/** Advance the fake clock, flushing timers AND promise microtasks. */
async function flush(ms = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

/**
 * Select the provider and click Start (fake-timer variant: no waitFor —
 * every async step is flushed by advancing the fake clock).
 */
async function submitStartForm() {
  fireEvent.mouseDown(screen.getByRole('combobox'))
  await flush(100)
  fireEvent.click(screen.getByRole('option', { name: 'OpenAI' }))
  fireEvent.click(screen.getByRole('button', { name: /^start$/i }))
  // Start mutation resolves → panel switches → immediate session check.
  await flush(0)
}

// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.useFakeTimers()
  mockState.startMutate.mockReset()
  mockState.getSession.mockReset()
  mockState.callbackMutate.mockReset()
  mockState.toastSuccess.mockReset()
  mockState.toastError.mockReset()
  mockState.getSession.mockResolvedValue(pendingSession())
  mockState.startMutate.mockResolvedValue(TEST_START_RESULT)
})

// ---------------------------------------------------------------------------

describe('OAuthStartDialog polling lifecycle (fake timers)', () => {
  it('polls on the interval until success, then closes with feedback', async () => {
    mockState.getSession
      .mockResolvedValueOnce(pendingSession()) // immediate attempt 1
      .mockResolvedValueOnce(successSession()) // attempt 2
    const onOpenChange = vi.fn()
    renderDialog(onOpenChange)

    await submitStartForm()

    // Pending panel is up, dialog still open.
    expect(
      screen.getAllByText('Waiting for authorization…').length
    ).toBeGreaterThan(0)
    expect(onOpenChange).not.toHaveBeenCalledWith(false)

    await flush(OAUTH_SESSION_POLL_INTERVAL_MS) // attempt 2 → success

    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(mockState.toastSuccess).toHaveBeenCalledWith(
      'Authorization completed.'
    )
  })

  it('shows an honest still-waiting state when the attempt budget runs out', async () => {
    mockState.getSession.mockResolvedValue(pendingSession())
    const onOpenChange = vi.fn()
    renderDialog(onOpenChange)

    await submitStartForm() // attempt 1

    for (let i = 1; i < OAUTH_SESSION_POLL_MAX_ATTEMPTS; i += 1) {
      await flush(OAUTH_SESSION_POLL_INTERVAL_MS)
    }

    // Honest exhaustion message with the attempt count — NOT a success.
    expect(
      screen.getByText(/Still waiting after 30 checks/)
    ).toBeInTheDocument()
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
    expect(mockState.toastSuccess).not.toHaveBeenCalled()
    // The "actively waiting" spinner row is gone (title remains).
    expect(
      screen.queryByText('Waiting for authorization…', { selector: 'span' })
    ).toBeNull()

    // The manual-callback entry remains available as the way forward.
    expect(
      screen.getByPlaceholderText(
        'Paste the callback URL from the OAuth redirect'
      )
    ).toBeEnabled()

    // Polling really stopped: no further checks after more intervals.
    const checksSoFar = mockState.getSession.mock.calls.length
    expect(checksSoFar).toBe(OAUTH_SESSION_POLL_MAX_ATTEMPTS)
    await flush(10 * OAUTH_SESSION_POLL_INTERVAL_MS)
    expect(mockState.getSession.mock.calls.length).toBe(checksSoFar)
  })

  it('surfaces a polled authorization error and stays open', async () => {
    mockState.getSession.mockResolvedValue({
      provider: 'openai',
      state: TEST_STATE,
      status: 'error',
      error: 'user denied access',
    })
    const onOpenChange = vi.fn()
    renderDialog(onOpenChange)

    await submitStartForm()

    expect(
      screen.getByText('Authorization failed: user denied access')
    ).toBeInTheDocument()
    expect(onOpenChange).not.toHaveBeenCalledWith(false)

    // An errored session is settled: no further checks fire.
    const checksSoFar = mockState.getSession.mock.calls.length
    await flush(3 * OAUTH_SESSION_POLL_INTERVAL_MS)
    expect(mockState.getSession.mock.calls.length).toBe(checksSoFar)
  })

  it('stops polling when the dialog closes while the session is pending', async () => {
    mockState.getSession.mockResolvedValue(pendingSession())
    const onOpenChange = vi.fn()
    const view = renderDialog(onOpenChange)

    await submitStartForm()
    expect(mockState.getSession.mock.calls.length).toBe(1)

    // The parent closes the dialog while pending (abandon confirmed
    // upstream); the pending-state cleanup must stop the poll loop.
    view.rerender(<OAuthStartDialog open={false} onOpenChange={onOpenChange} />)
    await flush(0)

    await flush(10 * OAUTH_SESSION_POLL_INTERVAL_MS)
    expect(mockState.getSession.mock.calls.length).toBe(1)
  })
})
