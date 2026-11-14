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
  updateRuntimeSettings: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getRuntimeSettings: vi.fn().mockResolvedValue({
      systemProxyUrl: 'http://127.0.0.1:7890',
      proxySessionChannelConcurrencyLimit: 7,
      proxySessionChannelQueueWaitMs: 2600,
      codexUpstreamWebsocketEnabled: false,
    }),
    updateRuntimeSettings: (...args: unknown[]) =>
      testState.updateRuntimeSettings(...args),
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
  testState.updateRuntimeSettings.mockReset()
  testState.updateRuntimeSettings.mockResolvedValue({})
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
  it('only edits supported controls without submitting legacy session settings', async () => {
    renderProxyTransportSection()
    const websocket = await screen.findByRole('checkbox', {
      name: /WebSocket/,
    })
    expect(
      screen.queryByRole('spinbutton', { name: /Session channel|会话通道/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('spinbutton', { name: /Queue wait|队列等待/ })
    ).not.toBeInTheDocument()
    expect(
      await screen.findByRole('button', { name: /Save|保存/ })
    ).toBeDisabled()
    fireEvent.click(websocket)
    fireEvent.click(screen.getByRole('button', { name: /Save|保存/ }))
    await waitFor(() => {
      expect(testState.updateRuntimeSettings).toHaveBeenCalledWith({
        codexUpstreamWebsocketEnabled: true,
      })
    })
  })

  it('retains the changed control and reports an async save failure', async () => {
    testState.updateRuntimeSettings.mockRejectedValue(
      new Error('fixture save failed')
    )
    renderProxyTransportSection()
    const websocket = await screen.findByRole('checkbox', { name: /WebSocket/ })
    fireEvent.click(websocket)
    fireEvent.click(screen.getByRole('button', { name: /Save|保存/ }))
    await waitFor(() => expect(testState.toastError).toHaveBeenCalled())
    expect(websocket).toBeChecked()
    expect(screen.getByRole('button', { name: /Save|保存/ })).toBeEnabled()
  })

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
