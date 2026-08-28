// Behavior tests for the operator-tunable status-code verdict fields
// (competitor-study-2026-08 P1-2): the routing section edits
// proxyRetryStatusRanges / proxyDisableStatusRanges. Loose shape validation
// catches obvious typos client-side; range bounds are enforced server-side.

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

import '@/i18n/config'

import { RoutingSection } from '../routing-section'

const testState = vi.hoisted(() => ({
  getRuntimeSettings: vi.fn(),
  updateRuntimeSettings: vi.fn(),
  toastSuccess: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getRuntimeSettings: (...args: unknown[]) =>
      testState.getRuntimeSettings(...args),
    updateRuntimeSettings: (...args: unknown[]) =>
      testState.updateRuntimeSettings(...args),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: testState.toastSuccess,
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
  },
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: React.ReactNode }) => <a>{props.children}</a>,
}))

vi.mock('../../../../components/form-navigation-guard', () => ({
  FormNavigationGuard: () => null,
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
  testState.getRuntimeSettings.mockReset()
  testState.updateRuntimeSettings.mockReset()
  testState.toastSuccess.mockReset()
  testState.getRuntimeSettings.mockResolvedValue({
    routingFallbackUnitCost: 1,
    tokenRouterFailureCooldownMaxSec: 2592000,
    proxyFirstByteTimeoutSec: 0,
    proxyRetryStatusRanges: '401,403,408,409,425,429,500-599',
    proxyDisableStatusRanges: '',
    disableCrossProtocolFallback: false,
    routingWeights: {
      baseWeightFactor: 0.5,
      valueScoreFactor: 0.5,
      costWeight: 0.4,
      balanceWeight: 0.3,
      usageWeight: 0.3,
    },
  })
})

afterEach(() => cleanup())

function renderRoutingSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <RoutingSection />
    </QueryClientProvider>
  )
}

describe('RoutingSection status-code verdict fields (P1-2)', () => {
  it('renders the server-sent specs in the two inputs', async () => {
    renderRoutingSection()

    const retryInput = (await screen.findByLabelText(
      'Retryable statuses'
    )) as HTMLInputElement
    const disableInput = (await screen.findByLabelText(
      'Auto-disable statuses'
    )) as HTMLInputElement
    expect(retryInput.value).toBe('401,403,408,409,425,429,500-599')
    expect(disableInput.value).toBe('')
  })

  it('blocks malformed specs before submitting', async () => {
    renderRoutingSection()

    const retryInput = (await screen.findByLabelText(
      'Retryable statuses'
    )) as HTMLInputElement
    fireEvent.change(retryInput, { target: { value: '5xx' } })

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(
        screen.getByText(
          'Invalid spec — use codes and ranges, e.g. 401 or 500-599'
        )
      ).toBeInTheDocument()
    })
    expect(testState.updateRuntimeSettings).not.toHaveBeenCalled()
  })

  it('submits the edited specs as part of the changed payload', async () => {
    testState.updateRuntimeSettings.mockResolvedValue({ success: true })
    renderRoutingSection()

    const disableInput = (await screen.findByLabelText(
      'Auto-disable statuses'
    )) as HTMLInputElement
    fireEvent.change(disableInput, { target: { value: '401' } })

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(testState.updateRuntimeSettings).toHaveBeenCalled()
    })
    const payload = testState.updateRuntimeSettings.mock.calls[0][0] as Record<
      string,
      unknown
    >
    expect(payload.proxyDisableStatusRanges).toBe('401')
    expect(payload.proxyRetryStatusRanges).toBeUndefined()
    await waitFor(() => {
      expect(testState.toastSuccess).toHaveBeenCalled()
    })
  })
})
