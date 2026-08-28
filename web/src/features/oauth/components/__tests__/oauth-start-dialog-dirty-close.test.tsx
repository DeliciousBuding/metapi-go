// Regression test: closing the OAuth start form with unsaved edits silently
// discarded them (Cancel / X / Escape all closed immediately). The form view
// must now route through the shared dirty-close guard (audit #1029 batch B);
// the pending panel keeps its own abandon confirmation.
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
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import { OAuthStartDialog } from '../oauth-start-dialog'

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

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

afterAll(() => {
  vi.restoreAllMocks()
})

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: ReactNode }) => <a>{children}</a>,
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
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
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useSubmitOAuthManualCallback: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
}))

vi.mock('../../lib/oauth-session-polling', () => ({
  useOAuthSessionPolling: () => ({
    session: undefined,
    exhausted: false,
    kick: vi.fn(),
  }),
  OAUTH_SESSION_POLL_MAX_ATTEMPTS: 100,
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

/** Select the first provider — makes the form dirty. */
async function makeFormDirty() {
  fireEvent.mouseDown(screen.getByRole('combobox'))
  const option = await screen.findByRole('option', { name: 'OpenAI' })
  fireEvent.click(option)
}

describe('OAuthStartDialog dirty-close guard', () => {
  it('asks for confirmation before closing a dirty form', async () => {
    const onOpenChange = vi.fn()
    renderDialog(onOpenChange)

    await makeFormDirty()
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))

    expect(
      await screen.findByText('Discard unsaved changes?')
    ).toBeInTheDocument()
    expect(onOpenChange).not.toHaveBeenCalled()
  })

  it('closes after confirming the discard', async () => {
    const onOpenChange = vi.fn()
    renderDialog(onOpenChange)

    await makeFormDirty()
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))
    await screen.findByText('Discard unsaved changes?')

    fireEvent.click(screen.getByRole('button', { name: 'Discard changes' }))

    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
  })

  it('keeps the form open when the discard is declined', async () => {
    const onOpenChange = vi.fn()
    renderDialog(onOpenChange)

    await makeFormDirty()
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))
    await screen.findByText('Discard unsaved changes?')

    fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }))

    expect(onOpenChange).not.toHaveBeenCalled()
    expect(screen.getByText(/Start authorization/)).toBeInTheDocument()
  })
})
