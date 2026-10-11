import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { ImportedUpstreamDetail } from '@/lib/api/imported-upstreams'
import { getPlatformDefinition } from '@/lib/platform-catalog'

import { UpstreamConnectionForm } from '../components/upstream-connection-form'
import { UpstreamIdentity } from '../components/upstream-identity'
import { connectionSchema } from '../lib/upstream-config'

const mocks = vi.hoisted(() => ({ update: vi.fn(), dirty: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { updateImportedUpstream: mocks.update } }))
vi.mock('@/lib/toast', () => ({ toast: { success: vi.fn() } }))

const detail: ImportedUpstreamDetail = {
  id: 4,
  name: 'Domestic model gateway',
  provider: 'generic',
  dialect: 'generic',
  originKey: '',
  enabled: true,
  baseUrl: 'https://gateway.example',
  endpointConfig: {
    chat: { url: 'https://chat.example/custom-chat', auth: 'x-api-key' },
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
const jsonModeNotice =
  'JSON Schema is sent as JSON Object without schema constraints.'
let client: QueryClient
function mount(node: React.ReactNode) {
  client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={client}>{node}</QueryClientProvider>
  )
}
async function chooseProfile(name: string) {
  fireEvent.click(screen.getByRole('combobox', { name: 'Protocol adapter' }))
  const option = await screen.findByRole('option', { name })
  fireEvent.pointerDown(option)
  fireEvent.click(option)
}
beforeEach(() => {
  vi.clearAllMocks()
  Element.prototype.scrollIntoView = vi.fn()
  mocks.update.mockResolvedValue({ success: true })
})
afterEach(() => {
  cleanup()
  client?.clear()
})

describe('domestic and aggregation adapters', () => {
  it.each([
    ['moonshot', 'Kimi'],
    ['longcat', 'LongCat'],
    ['openrouter', 'OpenRouter'],
    ['cerebras', 'Cerebras'],
    ['nanogpt', 'NanoGPT'],
  ])(
    'saves %s on a compatible custom provider without changing the endpoint URL',
    async (profile, label) => {
      mount(
        <UpstreamConnectionForm detail={detail} onDirtyChange={mocks.dirty} />
      )
      fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
      fireEvent.click(
        screen.getByRole('button', { name: 'Configure Chat endpoint' })
      )
      await chooseProfile(label)
      expect(
        screen.getByRole('combobox', { name: 'Authentication' })
      ).toHaveTextContent('Bearer')
      expect(
        screen.getByRole('combobox', { name: 'Authentication' })
      ).toBeDisabled()
      expect(
        screen.getByRole('checkbox', { name: 'Responses' })
      ).not.toBeChecked()
      expect(
        screen.getByRole('checkbox', { name: 'Messages' })
      ).not.toBeChecked()
      fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
      await waitFor(() =>
        expect(mocks.update).toHaveBeenCalledWith(4, {
          endpointConfig: {
            chat: {
              url: 'https://chat.example/custom-chat',
              auth: 'bearer',
              profile,
            },
          },
        })
      )
    }
  )

  it('shows the Kimi JSON mode limitation only while its endpoint adapter is being edited', async () => {
    mount(
      <UpstreamConnectionForm
        detail={{
          ...detail,
          provider: 'moonshot',
          endpointConfig: {
            chat: {
              url: 'https://api.moonshot.cn/v1/chat/completions',
              auth: 'bearer',
              profile: 'moonshot',
            },
          },
        }}
        onDirtyChange={mocks.dirty}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    expect(screen.queryByText(jsonModeNotice)).not.toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    )
    expect(screen.getByRole('note')).toHaveTextContent(jsonModeNotice)
    expect(
      screen.getByRole('combobox', { name: 'Protocol adapter' })
    ).toHaveAccessibleDescription(jsonModeNotice)
    await chooseProfile('Standard protocol')
    expect(screen.queryByText(jsonModeNotice)).not.toBeInTheDocument()
    expect(
      screen.getByRole('combobox', { name: 'Protocol adapter' })
    ).not.toHaveAttribute('aria-describedby')
    await chooseProfile('Kimi')
    expect(screen.getByText(jsonModeNotice)).toBeVisible()
  })

  it.each(['imageGeneration', 'imageEdit'] as const)(
    'saves OpenRouter image adaptation only for %s',
    async (key) => {
      mount(
        <UpstreamConnectionForm
          detail={{
            ...detail,
            endpointConfig: {
              chat: { url: 'https://chat.example/plain-chat', auth: 'bearer' },
              [key]: {
                url: 'https://openrouter.ai/api/v1/chat/completions',
                auth: 'x-api-key',
              },
            },
          }}
          onDirtyChange={mocks.dirty}
        />
      )
      fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
      fireEvent.click(
        screen.getByRole('button', { name: 'Images 1 / 4 configured' })
      )
      fireEvent.click(
        screen.getByRole('button', {
          name: /Configure Image (generation|edits) endpoint/,
        })
      )
      await chooseProfile('OpenRouter Images')
      expect(screen.queryByText(jsonModeNotice)).not.toBeInTheDocument()
      expect(
        screen.queryByLabelText('Codex request model')
      ).not.toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
      await waitFor(() =>
        expect(mocks.update).toHaveBeenCalledWith(4, {
          endpointConfig: {
            chat: { url: 'https://chat.example/plain-chat', auth: 'bearer' },
            [key]: {
              url: 'https://openrouter.ai/api/v1/chat/completions',
              auth: 'bearer',
              profile: 'openrouter-image',
            },
          },
        })
      )
    }
  )

  it('rejects using Chat adapters for other capabilities and OpenRouter image adaptation for image variations', () => {
    const url = 'https://gateway.example/endpoint'
    for (const profile of [
      'moonshot',
      'longcat',
      'openrouter',
      'cerebras',
      'nanogpt',
    ]) {
      expect(
        connectionSchema.shape.endpointConfig.safeParse({
          chat: { url, auth: 'bearer', profile },
        }).success
      ).toBe(true)
      expect(
        connectionSchema.shape.endpointConfig.safeParse({
          responses: { url, auth: 'bearer', profile },
        }).success
      ).toBe(false)
      expect(
        connectionSchema.shape.endpointConfig.safeParse({
          messages: { url, auth: 'bearer', profile },
        }).success
      ).toBe(false)
      expect(
        connectionSchema.shape.endpointConfig.safeParse({
          chat: { url, auth: 'x-api-key', profile },
        }).success
      ).toBe(false)
    }
    for (const key of ['imageGeneration', 'imageEdit']) {
      expect(
        connectionSchema.shape.endpointConfig.safeParse({
          [key]: { url, auth: 'bearer', profile: 'openrouter-image' },
        }).success
      ).toBe(true)
      expect(
        connectionSchema.shape.endpointConfig.safeParse({
          [key]: { url, auth: 'none', profile: 'openrouter-image' },
        }).success
      ).toBe(false)
    }
    expect(
      connectionSchema.shape.endpointConfig.safeParse({
        imageVariation: { url, auth: 'bearer', profile: 'openrouter-image' },
      }).success
    ).toBe(false)
    expect(
      connectionSchema.shape.endpointConfig.safeParse({
        chat: { url, auth: 'bearer', profile: 'openrouter-image' },
      }).success
    ).toBe(false)
  })

  it('keeps media-only and retained SenseTime identifiers out of the Chat adapter list', async () => {
    mount(
      <UpstreamConnectionForm detail={detail} onDirtyChange={mocks.dirty} />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    )
    fireEvent.click(screen.getByRole('combobox', { name: 'Protocol adapter' }))
    expect(
      await screen.findByRole('option', { name: 'OpenRouter' })
    ).toBeVisible()
    expect(
      screen.queryByRole('option', { name: 'OpenRouter Images' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: /SenseTime/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: 'OpenCode Go' })
    ).not.toBeInTheDocument()
    expect(getPlatformDefinition('sensetime')?.selectable).toBe(false)
  })

  it.each([
    ['longcat', 'LongCat', 'longcat-color'],
    ['longcat_anthropic', 'LongCat', 'longcat-color'],
    ['nanogpt', 'NanoGPT', 'nanogpt'],
    ['nanogpt_responses', 'NanoGPT', 'nanogpt'],
    ['openrouter', 'OpenRouter', 'openrouter'],
    ['cerebras', 'Cerebras', 'cerebras-brand-color'],
  ])('uses one formal brand capsule for %s', (provider, name, icon) => {
    render(<UpstreamIdentity provider={provider} dialect='generic' />)
    const capsule = screen.getByText(name).parentElement
    if (!capsule) throw new Error('Missing brand capsule')
    expect(within(capsule).getByRole('img', { hidden: true })).toHaveAttribute(
      'src',
      expect.stringContaining(icon)
    )
    expect(screen.queryByText(/Compatible/)).not.toBeInTheDocument()
  })
})
