// Behavior tests for the OAuth start-dialog pending-session flow.
//
// After `useStartOAuth` resolves, the dialog must NOT close — it switches to
// a pending panel that polls `useOAuthSession(state)` and exposes a
// manual-callback fallback. These tests verify: (1) the panel replaces the
// form after submit, (2) the manual-callback form calls
// `api.submitOAuthManualCallback`, and (3) a polled `status === 'success'`
// fires `onOpenChange(false)`.

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
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { OAuthStartDialog } from '../oauth-start-dialog'
import type { OAuthStartInstructions } from '../../types'

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

const TEST_INSTRUCTIONS: OAuthStartInstructions = {
  redirectUri: 'http://localhost:8080/callback',
  callbackPort: 8080,
  callbackPath: '/callback',
  manualCallbackDelayMs: 5000,
  sshTunnelCommand: 'ssh -L 8080:localhost:8080 user@host',
}

const TEST_START_RESULT = {
  provider: 'openai',
  state: TEST_STATE,
  authorizationUrl: 'https://provider.example.com/auth',
  instructions: TEST_INSTRUCTIONS,
}

const mockState = vi.hoisted(() => ({
  startMutate: vi.fn(),
  sessionData: null as null | {
    provider: string
    state: string
    status: 'pending' | 'success' | 'error'
    error?: string
  },
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
  useOAuthSession: (state: string | null) => ({
    data: state ? mockState.sessionData : null,
  }),
  useSubmitOAuthManualCallback: () => ({
    mutateAsync: mockState.callbackMutate,
    isPending: false,
  }),
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
  mockState.sessionData = {
    provider: 'openai',
    state: TEST_STATE,
    status: 'pending',
  }
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
    fireEvent.click(
      screen.getByRole('button', { name: /^submit callback$/i })
    )

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
    const onOpenChange = vi.fn()
    const { rerender } = renderDialog(onOpenChange)

    await submitStartForm()

    // Sanity: the pending panel is showing.
    await waitFor(() => {
      expect(
        screen.getAllByText('Waiting for authorization…').length
      ).toBeGreaterThan(0)
    })

    // Flip the polled session to success and re-render so the mock
    // `useOAuthSession` returns the updated status.
    mockState.sessionData = {
      provider: 'openai',
      state: TEST_STATE,
      status: 'success',
    }
    rerender(<OAuthStartDialog open onOpenChange={onOpenChange} />)

    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
    expect(mockState.toastSuccess).toHaveBeenCalledWith(
      'Authorization completed.'
    )
  })
})
