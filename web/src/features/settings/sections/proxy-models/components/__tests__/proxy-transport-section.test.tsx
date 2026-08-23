// Honesty test for the system-proxy connectivity probe.
//
// POST /api/settings/system-proxy/test reports an unreachable proxy as
// HTTP 200 + `{success:false, ok:false, reachable:false, message}` — the
// probe outcome lives in the envelope, not the status code. The section must
// surface the probe's real verdict (error toast with the probe message) and
// never toast "Proxy reachable" for a failed probe.

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

import { ProxyTransportSection } from '../proxy-transport-section'

const testState = vi.hoisted(() => ({
  testSystemProxy: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getRuntimeSettings: vi.fn().mockResolvedValue({
      systemProxyUrl: 'http://127.0.0.1:7890',
    }),
    updateRuntimeSettings: vi.fn(),
    testSystemProxy: (...args: unknown[]) => testState.testSystemProxy(...args),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: testState.toastSuccess,
    error: testState.toastError,
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

vi.mock('../../../../components/settings-form-actions', () => ({
  SettingsFormActions: () => null,
}))

beforeAll(() => {
  // base-ui primitives query matchMedia under jsdom.
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
  testState.testSystemProxy.mockReset()
  testState.toastSuccess.mockReset()
  testState.toastError.mockReset()
})

afterEach(() => cleanup())

function renderProxyTransportSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ProxyTransportSection />
    </QueryClientProvider>
  )
}

describe('ProxyTransportSection — probe honesty', () => {
  it('reports the real failure when the probe cannot reach the proxy', async () => {
    testState.testSystemProxy.mockResolvedValue({
      success: false,
      ok: false,
      reachable: false,
      latencyMs: 0,
      proxyUrl: 'http://127.0.0.1:7890',
      targetUrl: 'https://www.gstatic.com/generate_204',
      message:
        'Get "https://www.gstatic.com/generate_204": proxyconnect tcp: dial tcp 127.0.0.1:7890: connect: connection refused',
    })

    renderProxyTransportSection()

    fireEvent.click(await screen.findByRole('button', { name: /Test|测试/ }))

    await waitFor(() => {
      expect(testState.toastError).toHaveBeenCalledTimes(1)
    })
    expect(String(testState.toastError.mock.calls[0]?.[0])).toMatch(
      /connection refused/
    )
    expect(testState.toastSuccess).not.toHaveBeenCalled()
  })

  it('toasts success only when the probe actually succeeded', async () => {
    testState.testSystemProxy.mockResolvedValue({
      success: true,
      ok: true,
      reachable: true,
      latencyMs: 88,
      proxyUrl: 'http://127.0.0.1:7890',
      targetUrl: 'https://www.gstatic.com/generate_204',
    })

    renderProxyTransportSection()

    fireEvent.click(await screen.findByRole('button', { name: /Test|测试/ }))

    await waitFor(() => {
      expect(testState.toastSuccess).toHaveBeenCalledTimes(1)
    })
    expect(testState.toastError).not.toHaveBeenCalled()
  })
})
