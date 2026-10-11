import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { ImportedUpstreamDetail } from '@/lib/api/imported-upstreams'

import { UpstreamConnectionForm } from '../components/upstream-connection-form'

const mocks = vi.hoisted(() => ({ update: vi.fn(), requestConfig: vi.fn() }))
vi.mock('@/lib/api', () => ({
  api: {
    updateImportedUpstream: mocks.update,
    getImportedRequestConfig: mocks.requestConfig,
  },
}))
vi.mock('@/lib/toast', () => ({ toast: { success: vi.fn() } }))
const detail: ImportedUpstreamDetail = {
  id: 4,
  name: 'New API',
  provider: 'new-api',
  dialect: 'generic',
  originKey: '',
  enabled: true,
  baseUrl: 'https://gateway.example',
  endpointConfig: {
    chat: {
      url: 'https://gateway.example/v1/chat/completions',
      auth: 'bearer',
    },
  },
  useSystemProxy: false,
  hasChannelProxy: false,
  hasCustomHeaders: false,
  hasParamOverride: false,
  channelProxyDisplay: '',
  openaiChatCompletionPath: '',
  openaiResponsePath: '',
  anthropicMessagePath: '',
}
let client: QueryClient
beforeEach(() => {
  vi.clearAllMocks()
  Element.prototype.scrollIntoView = vi.fn()
  mocks.update.mockResolvedValue({ success: true })
})
afterEach(() => {
  cleanup()
  client?.clear()
})
function mount() {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  render(
    <QueryClientProvider client={client}>
      <UpstreamConnectionForm detail={detail} onDirtyChange={vi.fn()} />
    </QueryClientProvider>
  )
}
describe('advanced upstream connection settings', () => {
  it('keeps protocols and request settings out of normal connection editing', async () => {
    mount()
    expect(
      screen.getByRole('button', { name: 'Advanced settings' })
    ).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.queryByRole('checkbox', { name: 'Chat' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Edit request settings' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('switch', { name: 'System proxy' })
    ).not.toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'My gateway' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith(4, { name: 'My gateway' })
    )
    expect(mocks.requestConfig).not.toHaveBeenCalled()
  })
  it('reveals an invalid advanced endpoint when a hidden draft is submitted', async () => {
    mount()
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    expect(mocks.requestConfig).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    )
    fireEvent.change(screen.getByLabelText('Endpoint URL'), {
      target: { value: 'invalid' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Advanced settings' })
      ).toHaveAttribute('aria-expanded', 'true')
    )
    expect(screen.getByLabelText('Endpoint URL')).toBeVisible()
    expect(screen.getByLabelText('Endpoint URL')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(mocks.update).not.toHaveBeenCalled()
    fireEvent.change(screen.getByLabelText('Endpoint URL'), {
      target: { value: 'https://gateway.example/custom/chat' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          chat: { url: 'https://gateway.example/custom/chat', auth: 'bearer' },
        },
      })
    )
  })
})
