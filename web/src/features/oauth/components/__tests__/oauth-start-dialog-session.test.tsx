// Behavior tests for the OAuth start-dialog pending-session flow.
//
// After `useStartOAuth` resolves, the dialog must NOT close — it switches to
// a pending panel that shows the returned `instructions` + `state`
// (copyable), polls the session via `useOAuthSessionPolling(state)` and
// exposes a validated manual-callback fallback. These tests verify:
// (1) the panel replaces the form after submit, (2) the panel surfaces the
// state + instructions, (3) the manual-callback form validates the URL and
// reports success/failure explicitly, and (4) a polled
// `status === 'success'` fires `onOpenChange(false)`.
// Fake-timer polling behavior lives in oauth-start-dialog-polling.test.tsx.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
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
  vi.clearAllMocks()
})

afterAll(() => {
  vi.restoreAllMocks()
})

// ---------------------------------------------------------------------------
// Mock fixtures
// ---------------------------------------------------------------------------

const TEST_STATE = 'test-session-state'

const TEST_SSH_TUNNEL_COMMAND = 'ssh -L 8080:localhost:8080 user@host'
const TEST_SSH_TUNNEL_KEY_COMMAND =
  'ssh -i <path_to_your_key> -L 8080:localhost:8080 user@host'

const TEST_INSTRUCTIONS: OAuthStartInstructions = {
  redirectUri: 'http://localhost:8080/callback',
  callbackPort: 8080,
  callbackPath: '/callback',
  manualCallbackDelayMs: 5000,
  sshTunnelCommand: TEST_SSH_TUNNEL_COMMAND,
  sshTunnelKeyCommand: TEST_SSH_TUNNEL_KEY_COMMAND,
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

/**
 * Select the first provider in the start-authorization form and click the
 * Start button. `useStartOAuth` is mocked to resolve immediately with
 * `TEST_START_RESULT`, which flips the dialog into the pending panel.
 */
async function submitStartForm() {
  // Open the provider <Select> dropdown and pick the first provider.
  fireEvent.mouseDown(screen.getByRole('combobox'))
  const option = await screen.findByRole('option', { name: 'OpenAI' })
  fireEvent.click(option)

  // Click the "Start" submit button (the form's primary action).
  fireEvent.click(screen.getByRole('button', { name: /^start$/i }))

  await waitFor(() => expect(mockState.startMutate).toHaveBeenCalledTimes(1))
}

// ---------------------------------------------------------------------------

beforeEach(() => {
  mockState.startMutate.mockReset()
  mockState.callbackMutate.mockReset()
  mockState.getSession.mockReset()
  // Default: the polled session stays pending (the polling is bounded and
  // keeps the dialog open; fake-timer exhaustion lives in the polling file).
  mockState.getSession.mockResolvedValue({
    provider: 'openai',
    state: TEST_STATE,
    status: 'pending',
  })
  // Default: start mutation resolves with the test result.
  mockState.startMutate.mockResolvedValue(TEST_START_RESULT)
  mockState.toastSuccess.mockReset()
  mockState.toastError.mockReset()
})

// ---------------------------------------------------------------------------
// 1. After submit, the dialog stays open and shows the pending panel
// ---------------------------------------------------------------------------

describe('OAuthStartDialog pending session', () => {
  it('switches to the pending panel instead of closing after submit', async () => {
    const onOpenChange = vi.fn()
    renderDialog(onOpenChange)

    await submitStartForm()

    // The dialog did NOT close (onOpenChange(false) was not called by the
    // start-authorization path — only the session-success path closes).
    expect(onOpenChange).not.toHaveBeenCalledWith(false)

    // The pending panel is rendered: the "Waiting for authorization…"
    // title is visible, and the form's "Start" button is gone.
    await waitFor(() => {
      expect(
        screen.getAllByText('Waiting for authorization…').length
      ).toBeGreaterThan(0)
    })
    expect(
      screen.queryByRole('button', { name: /^start$/i })
    ).not.toBeInTheDocument()
  })

  // -------------------------------------------------------------------------
  // 2. Manual callback submit calls api.submitOAuthManualCallback
  // -------------------------------------------------------------------------

  it('calls the manual-callback mutation with state + pasted URL', async () => {
    const onOpenChange = vi.fn()
    renderDialog(onOpenChange)

    await submitStartForm()

    // Type a callback URL into the manual-callback input.
    const input = await screen.findByPlaceholderText(
      'Paste the callback URL from the OAuth redirect'
    )
    fireEvent.change(input, {
      target: { value: 'http://localhost:8080/callback?code=abc&state=test' },
    })

    // Click the "Submit callback" button.
    fireEvent.click(screen.getByRole('button', { name: /^submit callback$/i }))

    await waitFor(() =>
      expect(mockState.callbackMutate).toHaveBeenCalledTimes(1)
    )
    expect(mockState.callbackMutate).toHaveBeenCalledWith({
      state: TEST_STATE,
      callbackUrl: 'http://localhost:8080/callback?code=abc&state=test',
    })
  })

  // -------------------------------------------------------------------------
  // 3. On session status === 'success', the dialog closes
  // -------------------------------------------------------------------------

  it('closes the dialog when the polled session reports success', async () => {
    // The immediate post-start check already reports success.
    mockState.getSession.mockResolvedValue({
      provider: 'openai',
      state: TEST_STATE,
      status: 'success',
    })
    const onOpenChange = vi.fn()
    renderDialog(onOpenChange)

    await submitStartForm()

    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
    expect(mockState.toastSuccess).toHaveBeenCalledWith(
      'Authorization completed.'
    )
  })

  // -------------------------------------------------------------------------
  // 4. Closing while pending requires the abandon confirmation (#889)
  // -------------------------------------------------------------------------

  it('asks for confirmation when closing while the session is pending', async () => {
    const onOpenChange = vi.fn()
    renderDialog(onOpenChange)

    await submitStartForm()

    // Sanity: the pending panel is showing (its footer Cancel button).
    await waitFor(() => {
      expect(
        screen.getAllByText('Waiting for authorization…').length
      ).toBeGreaterThan(0)
    })

    // Closing via the pending panel's Cancel must NOT close the dialog
    // directly — it surfaces the abandon confirmation instead.
    fireEvent.click(screen.getByRole('button', { name: /^cancel$/i }))
    expect(onOpenChange).not.toHaveBeenCalledWith(false)

    // The abandon confirmation is visible.
    expect(await screen.findByText('Abort authorization?')).toBeInTheDocument()

    // Keeping the wait dismisses the confirmation and leaves the dialog open.
    fireEvent.click(screen.getByRole('button', { name: 'Keep waiting' }))
    await waitFor(() => {
      expect(screen.queryByText('Abort authorization?')).toBeNull()
    })
    expect(onOpenChange).not.toHaveBeenCalledWith(false)

    // Re-opening the confirmation and confirming the abandon finally closes.
    fireEvent.click(screen.getByRole('button', { name: /^cancel$/i }))
    await screen.findByText('Abort authorization?')
    fireEvent.click(screen.getByRole('button', { name: /^abort$/i }))
    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
  })

  // -------------------------------------------------------------------------
  // 5. The pending panel surfaces state + instructions (copyable)
  // -------------------------------------------------------------------------

  it('shows the session state and instructions with copy affordances', async () => {
    renderDialog(vi.fn())

    await submitStartForm()

    // Session state + instructions text are rendered (not dropped).
    expect(await screen.findByText(TEST_STATE)).toBeInTheDocument()
    expect(screen.getByText(TEST_INSTRUCTIONS.redirectUri)).toBeInTheDocument()
    expect(screen.getByText(TEST_SSH_TUNNEL_COMMAND)).toBeInTheDocument()
    expect(screen.getByText(TEST_SSH_TUNNEL_KEY_COMMAND)).toBeInTheDocument()

    // Every surfaced value has a copy button (state, redirect URI, tunnel,
    // tunnel-key variant).
    expect(
      screen.getAllByRole('button', { name: 'Copy' }).length
    ).toBeGreaterThanOrEqual(4)
  })

  // -------------------------------------------------------------------------
  // 6. Manual callback success: explicit feedback + immediate re-check
  // -------------------------------------------------------------------------

  it('confirms a submitted manual callback and re-checks the session', async () => {
    renderDialog(vi.fn())

    await submitStartForm()

    const input = await screen.findByPlaceholderText(
      'Paste the callback URL from the OAuth redirect'
    )
    await waitFor(() =>
      expect(mockState.getSession.mock.calls.length).toBeGreaterThan(0)
    )
    const checksBefore = mockState.getSession.mock.calls.length

    fireEvent.change(input, {
      target: { value: 'http://localhost:8080/callback?code=abc&state=test' },
    })
    fireEvent.click(screen.getByRole('button', { name: /^submit callback$/i }))

    await waitFor(() =>
      expect(mockState.callbackMutate).toHaveBeenCalledTimes(1)
    )
    // Explicit success feedback…
    await waitFor(() =>
      expect(mockState.toastSuccess).toHaveBeenCalledWith(
        'Callback submitted — verifying the authorization…'
      )
    )
    // …the input is cleared…
    expect((input as HTMLInputElement).value).toBe('')
    // …and polling kicks an immediate session re-check (no interval wait).
    await waitFor(() =>
      expect(mockState.getSession.mock.calls.length).toBeGreaterThan(
        checksBefore
      )
    )
  })

  // -------------------------------------------------------------------------
  // 7. Manual callback failure: the backend error is surfaced, input kept
  // -------------------------------------------------------------------------

  it('surfaces the backend error when the manual callback is rejected', async () => {
    mockState.callbackMutate.mockRejectedValueOnce(new Error('session expired'))
    renderDialog(vi.fn())

    await submitStartForm()

    const input = await screen.findByPlaceholderText(
      'Paste the callback URL from the OAuth redirect'
    )
    fireEvent.change(input, {
      target: { value: 'http://localhost:8080/callback?code=stale' },
    })
    fireEvent.click(screen.getByRole('button', { name: /^submit callback$/i }))

    await waitFor(() =>
      expect(mockState.toastError).toHaveBeenCalledWith('session expired')
    )
    // The pasted URL is kept so the user can correct and resubmit.
    expect((input as HTMLInputElement).value).toBe(
      'http://localhost:8080/callback?code=stale'
    )
  })

  // -------------------------------------------------------------------------
  // 8. Manual callback validation: non-http(s) URLs are rejected inline
  // -------------------------------------------------------------------------

  it('rejects a manual callback that is not a valid http(s) URL', async () => {
    renderDialog(vi.fn())

    await submitStartForm()

    const input = await screen.findByPlaceholderText(
      'Paste the callback URL from the OAuth redirect'
    )
    const submit = screen.getByRole('button', { name: /^submit callback$/i })

    // Not a URL at all.
    fireEvent.change(input, { target: { value: 'not a url' } })
    fireEvent.click(submit)
    expect(mockState.callbackMutate).not.toHaveBeenCalled()
    expect(
      screen.getByText('Enter a valid http(s) callback URL.')
    ).toBeInTheDocument()

    // A parseable URL with a non-http(s) protocol is rejected too.
    fireEvent.change(input, { target: { value: 'javascript:alert(1)' } })
    fireEvent.click(submit)
    expect(mockState.callbackMutate).not.toHaveBeenCalled()

    // Editing the field clears the inline error.
    fireEvent.change(input, {
      target: { value: 'http://localhost:8080/callback?code=ok' },
    })
    expect(screen.queryByText('Enter a valid http(s) callback URL.')).toBeNull()
  })
})
