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
import type {
  ImportedEndpointConfig,
  ImportedUpstreamDetail,
} from '@/lib/api/imported-upstreams'

import { UpstreamConnectionForm } from '../components/upstream-connection-form'
import { UpstreamIdentity } from '../components/upstream-identity'
import { connectionSchema, connectionValues } from '../lib/upstream-config'

const mocks = vi.hoisted(() => ({
  update: vi.fn(),
  dirty: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: {
    updateImportedUpstream: mocks.update,
  },
}))
vi.mock('@/lib/toast', () => ({ toast: { success: vi.fn() } }))

const modelWireUrls = {
  responses: 'https://responses.example/custom-responses',
  messages: 'https://messages.example/custom-messages',
}
const endpoints: ImportedEndpointConfig = {
  chat: {
    url: 'https://chat.example/custom-chat',
    auth: 'bearer',
    profile: 'opencode-go',
    modelWireUrls,
  },
}
const detail: ImportedUpstreamDetail = {
  id: 4,
  name: 'OpenCode Go',
  provider: 'opencode_go',
  dialect: 'generic',
  originKey: '',
  enabled: true,
  baseUrl: 'https://gateway.example',
  endpointConfig: endpoints,
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

describe('provider Chat wire adapters', () => {
  it.each(['generic', 'cline', 'bailian', 'opencode_go_anthropic'])(
    'does not offer the dynamic OpenCode Go adapter for %s',
    async (provider) => {
      mount(
        <UpstreamConnectionForm
          detail={{
            ...detail,
            provider,
            endpointConfig: {
              chat: { url: 'https://chat.example/api', auth: 'bearer' },
            },
          }}
          onDirtyChange={mocks.dirty}
        />
      )
      fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
      fireEvent.click(
        screen.getByRole('button', { name: 'Configure Chat endpoint' })
      )
      fireEvent.click(
        screen.getByRole('combobox', { name: 'Protocol adapter' })
      )
      expect(await screen.findByRole('option', { name: 'Cline' })).toBeVisible()
      expect(screen.getByRole('option', { name: 'Bailian' })).toBeVisible()
      expect(
        screen.queryByRole('option', { name: 'OpenCode Go' })
      ).not.toBeInTheDocument()
    }
  )

  it('accepts complete provider adapters and rejects missing, misplaced or unsafe internal destinations', () => {
    for (const profile of ['bailian', 'cline', 'opencode-go']) {
      const extra = profile === 'opencode-go' ? { modelWireUrls } : {}
      expect(
        connectionSchema.shape.endpointConfig.safeParse({
          chat: {
            url: 'https://chat.example/api',
            auth: 'bearer',
            profile,
            ...extra,
          },
        }).success
      ).toBe(true)
      expect(
        connectionSchema.shape.endpointConfig.safeParse({
          messages: {
            url: 'https://chat.example/api',
            auth: 'bearer',
            profile,
            ...extra,
          },
        }).success
      ).toBe(false)
      expect(
        connectionSchema.shape.endpointConfig.safeParse({
          chat: {
            url: 'https://chat.example/api',
            auth: 'x-api-key',
            profile,
            ...extra,
          },
        }).success
      ).toBe(false)
    }
    for (const chat of [
      { ...endpoints.chat, modelWireUrls: undefined },
      { ...endpoints.chat, profile: 'cline' },
      {
        ...endpoints.chat,
        modelWireUrls: { responses: modelWireUrls.responses },
      },
      ...[
        '',
        'invalid',
        'file:///tmp/api',
        Object.assign(new URL('https://example.com/api'), {
          username: 'fixture-user',
          password: 'fixture-password',
        }).href,
        'https://example.com/api?token=fixture',
        'https://example.com/api#fragment',
      ].map((messages) => ({
        ...endpoints.chat,
        modelWireUrls: { ...modelWireUrls, messages },
      })),
    ]) {
      expect(
        connectionSchema.shape.endpointConfig.safeParse({ chat }).success
      ).toBe(false)
    }
  })

  it.each([
    ['bailian', 'Bailian'],
    ['cline', 'Cline'],
  ])('saves the %s Chat adapter using Bearer', async (profile, label) => {
    mount(
      <UpstreamConnectionForm
        detail={{
          ...detail,
          endpointConfig: {
            chat: { url: 'https://chat.example/api', auth: 'x-api-key' },
          },
        }}
        onDirtyChange={mocks.dirty}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    )
    await chooseProfile(label)
    expect(
      screen.queryByRole('button', { name: 'OpenCode Go adapter URLs' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('combobox', { name: 'Authentication' })
    ).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          chat: { url: 'https://chat.example/api', auth: 'bearer', profile },
        },
      })
    )
  })

  it('edits one internal URL without replacing independent top-level endpoint URLs', async () => {
    const separate: ImportedEndpointConfig = {
      ...endpoints,
      responses: {
        url: 'https://public.example/independent-responses',
        auth: 'bearer',
      },
      messages: {
        url: 'https://public.example/independent-messages',
        auth: 'x-api-key',
      },
    }
    mount(
      <UpstreamConnectionForm
        detail={{ ...detail, endpointConfig: separate }}
        onDirtyChange={mocks.dirty}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'OpenCode Go adapter URLs' })
    )
    fireEvent.change(screen.getByLabelText('Responses wire URL'), {
      target: { value: 'https://responses.example/operator-path' },
    })
    fireEvent.click(
      screen.getByRole('button', { name: 'OpenCode Go adapter URLs' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          ...separate,
          chat: {
            ...separate.chat,
            modelWireUrls: {
              ...modelWireUrls,
              responses: 'https://responses.example/operator-path',
            },
          },
        },
      })
    )
    const result = connectionSchema.parse(
      connectionValues({ ...detail, endpointConfig: separate })
    )
    expect(result.endpointConfig).toEqual(separate)
  })

  it('requires explicit internal URLs on manual selection and reopens invalid collapsed fields', async () => {
    const separate: ImportedEndpointConfig = {
      chat: { url: 'https://chat.example/manual-path', auth: 'bearer' },
      responses: { url: 'https://top.example/responses', auth: 'bearer' },
      messages: { url: 'https://top.example/messages', auth: 'x-api-key' },
    }
    mount(
      <UpstreamConnectionForm
        detail={{ ...detail, endpointConfig: separate }}
        onDirtyChange={mocks.dirty}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    )
    await chooseProfile('OpenCode Go')
    expect(screen.getByLabelText('Responses wire URL')).toHaveValue('')
    expect(screen.getByLabelText('Messages wire URL')).toHaveValue('')
    fireEvent.click(
      screen.getByRole('button', { name: 'OpenCode Go adapter URLs' })
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(screen.getByLabelText('Responses wire URL')).toBeVisible()
    )
    expect(screen.getByLabelText('Responses wire URL')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    expect(mocks.update).not.toHaveBeenCalled()
    for (const [wire, url] of Object.entries(modelWireUrls)) {
      fireEvent.change(
        screen.getByLabelText(
          `${wire === 'responses' ? 'Responses' : 'Messages'} wire URL`
        ),
        { target: { value: url } }
      )
    }
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          ...separate,
          chat: { ...separate.chat, profile: 'opencode-go', modelWireUrls },
        },
      })
    )
  })

  it('removes incompatible internal destinations when changing to another adapter', async () => {
    mount(
      <UpstreamConnectionForm detail={detail} onDirtyChange={mocks.dirty} />
    )
    fireEvent.click(screen.getByRole('button', { name: 'Advanced settings' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Configure Chat endpoint' })
    )
    await chooseProfile('Cline')
    expect(
      screen.queryByRole('button', { name: 'OpenCode Go adapter URLs' })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() =>
      expect(mocks.update).toHaveBeenCalledWith(4, {
        endpointConfig: {
          chat: {
            url: endpoints.chat?.url,
            auth: 'bearer',
            profile: 'cline',
            modelWireUrls: undefined,
          },
        },
      })
    )
  })

  it('renders official Cline and OpenCode Go names with local brand icons', () => {
    render(
      <>
        <UpstreamIdentity provider='opencode_go' dialect='generic' />
        <UpstreamIdentity provider='cline' dialect='generic' />
      </>
    )
    for (const [name, icon] of [
      ['OpenCode Go', 'opencode'],
      ['Cline', 'cline'],
    ]) {
      const capsule = screen.getByText(name).parentElement
      expect(capsule).not.toBeNull()
      if (!capsule) throw new Error('Missing brand capsule')
      expect(
        within(capsule).getByRole('img', { hidden: true })
      ).toHaveAttribute('src', expect.stringContaining(icon))
    }
  })
})
