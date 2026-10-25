import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'
import type {
  ImportedEndpoint,
  ImportedEndpointConfig,
  ImportedUpstreamDetail,
} from '@/lib/api/imported-upstreams'

import { UpstreamConnectionForm } from '../components/upstream-connection-form'
import { connectionSchema } from '../lib/upstream-config'

const mocks = vi.hoisted(() => ({ update: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { updateImportedUpstream: mocks.update } }))
vi.mock('@/lib/toast', () => ({ toast: { success: vi.fn() } }))

const cases: Array<{
  provider: string
  key: keyof ImportedEndpointConfig
  name: string
  adapter: string
  endpoint: ImportedEndpoint
}> = [
  {
    provider: 'gemini_vertex',
    key: 'gemini',
    name: 'Gemini',
    adapter: 'Google Vertex AI',
    endpoint: {
      url: 'https://vertex.example/publishers/google/models',
      auth: 'bearer',
      profile: 'vertex',
      modelPath: true,
    },
  },
  {
    provider: 'gemini_vertex',
    key: 'geminiEmbeddings',
    name: 'Gemini embeddings',
    adapter: 'Google Vertex AI',
    endpoint: {
      url: 'https://vertex.example/publishers/google/models',
      auth: 'bearer',
      profile: 'vertex',
      modelPath: true,
    },
  },
  {
    provider: 'anthropic_gcp',
    key: 'messages',
    name: 'Messages',
    adapter: 'Anthropic on Vertex',
    endpoint: {
      url: 'https://vertex.example/publishers/anthropic/models',
      auth: 'bearer',
      profile: 'vertex-anthropic',
      modelPath: true,
    },
  },
  {
    provider: 'xai',
    key: 'responses',
    name: 'Responses',
    adapter: 'xAI subscription',
    endpoint: {
      url: 'https://subscription.example/responses',
      auth: 'bearer',
      profile: 'xai-subscription',
    },
  },
  {
    provider: 'github_copilot',
    key: 'chat',
    name: 'Chat',
    adapter: 'GitHub Copilot',
    endpoint: {
      url: 'https://copilot.example/chat/completions',
      auth: 'bearer',
      profile: 'github-copilot',
    },
  },
]
let client: QueryClient
beforeEach(async () => {
  await i18n.changeLanguage('en')
  vi.clearAllMocks()
  Element.prototype.scrollIntoView = vi.fn()
  mocks.update.mockResolvedValue({ success: true })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
})
afterEach(() => {
  cleanup()
  client.clear()
})

describe('imported endpoint profile compatibility', () => {
  it.each(cases)(
    'edits $provider $key without discarding its profile or model path',
    async ({ provider, key, name, adapter, endpoint }) => {
      const detail: ImportedUpstreamDetail = {
        id: 4,
        name: 'Imported connection',
        originKey: 'fixture',
        ownership: 'imported',
        provider,
        dialect: 'generic',
        enabled: true,
        baseUrl: 'https://provider.example',
        endpointConfig: { [key]: endpoint },
        useSystemProxy: false,
        hasChannelProxy: true,
        hasCustomHeaders: true,
        hasParamOverride: true,
        channelProxyDisplay: '',
        openaiChatCompletionPath: '',
        openaiResponsePath: '',
        anthropicMessagePath: '',
      }
      render(
        <QueryClientProvider client={client}>
          <UpstreamConnectionForm detail={detail} onDirtyChange={vi.fn()} />
        </QueryClientProvider>
      )
      fireEvent.click(
        screen.getByRole('button', { name: `Configure ${name} endpoint` })
      )
      expect(
        screen.getByRole('combobox', { name: 'Protocol adapter' })
      ).toHaveTextContent(adapter)
      if (endpoint.profile === 'vertex') {
        expect(
          screen.getByRole('checkbox', {
            name: 'Append model name and operation to the URL',
          })
        ).toHaveAttribute('aria-disabled', 'true')
      }
      const url = endpoint.modelPath
        ? screen.getByLabelText('Model URL prefix (ending in /models)')
        : screen.getByLabelText('Endpoint URL')
      fireEvent.change(url, {
        target: { value: 'https://custom.example/models' },
      })
      fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
      await waitFor(() =>
        expect(mocks.update).toHaveBeenCalledWith(4, {
          endpointConfig: {
            [key]: { ...endpoint, url: 'https://custom.example/models' },
          },
        })
      )
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    }
  )

  it('rejects unknown profiles, incorrect protocol slots and missing Vertex model paths', () => {
    const url = 'https://provider.example/models'
    const invalid = [
      { chat: { url, auth: 'bearer', profile: 'unknown-adapter' } },
      { gemini: { url, auth: 'bearer', profile: 'vertex' } },
      {
        geminiEmbeddings: {
          url,
          auth: 'x-goog-api-key',
          profile: 'vertex',
          modelPath: true,
        },
      },
      {
        messages: {
          url,
          auth: 'bearer',
          profile: 'vertex-anthropic',
          modelPath: false,
        },
      },
      {
        responses: {
          url,
          auth: 'bearer',
          profile: 'vertex-anthropic',
          modelPath: true,
        },
      },
      { chat: { url, auth: 'bearer', profile: 'xai-subscription' } },
      { responses: { url, auth: 'x-api-key', profile: 'xai-subscription' } },
      { messages: { url, auth: 'bearer', profile: 'github-copilot' } },
    ]
    for (const endpoints of invalid) {
      expect(
        connectionSchema.shape.endpointConfig.safeParse(endpoints).success
      ).toBe(false)
    }
  })
})
