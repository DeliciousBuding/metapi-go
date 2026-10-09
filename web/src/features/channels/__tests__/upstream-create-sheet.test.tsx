import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import i18n from 'i18next'
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
import type { ImportedEndpointConfig } from '@/lib/api/imported-upstreams'
import type {
  ResolvedUpstreamPreset,
  UpstreamPreset,
} from '@/lib/api/upstream-presets'

import { UpstreamCreateSheet } from '../components/upstream-create-sheet'

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  list: vi.fn(),
  resolve: vi.fn(),
}))
vi.mock('@/lib/api', () => ({ api: { createUpstreamChannel: mocks.create } }))
vi.mock('@/lib/api/upstream-presets', () => ({
  upstreamPresetsApi: {
    getUpstreamPresets: mocks.list,
    resolveUpstreamPreset: mocks.resolve,
  },
}))

const presets: UpstreamPreset[] = [
  {
    id: 'new-api-connection',
    name: 'New API',
    label: 'New API',
    provider: 'new-api',
    platform: 'new-api',
    group: 'gateway',
    defaultUrl: '',
    protocols: ['chat', 'responses', 'messages'],
    recommendedModels: [],
  },
  {
    id: 'deepseek-openai',
    name: 'DeepSeek',
    label: 'DeepSeek / OpenAI',
    provider: 'deepseek',
    platform: 'openai',
    group: 'domestic',
    defaultUrl: 'https://api.deepseek.com/v1',
    protocols: ['chat'],
    recommendedModels: ['deepseek-chat'],
  },
  {
    id: 'codingplan-claude',
    name: 'Aliyun CodingPlan',
    label: 'Aliyun CodingPlan / Claude',
    provider: 'bailian_anthropic',
    platform: 'claude',
    group: 'coding',
    defaultUrl: 'https://coding.dashscope.aliyuncs.com/apps/anthropic',
    protocols: ['messages'],
    recommendedModels: ['fixture-model'],
  },
  {
    id: 'openai-api',
    name: 'OpenAI',
    label: 'OpenAI API',
    provider: 'openai',
    platform: 'openai',
    group: 'other',
    defaultUrl: 'https://api.openai.com',
    protocols: ['chat', 'responses'],
    recommendedModels: [],
  },
]
const originalScroll = HTMLElement.prototype.scrollIntoView
const originalMedia = window.matchMedia
beforeAll(async () => {
  await i18n.changeLanguage('en')
  HTMLElement.prototype.scrollIntoView = vi.fn()
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }))
})
afterAll(() => {
  vi.unstubAllGlobals()
  HTMLElement.prototype.scrollIntoView = originalScroll
  window.matchMedia = originalMedia
})
beforeEach(() => {
  vi.clearAllMocks()
  mocks.create.mockReset().mockResolvedValue({ id: 42 })
  mocks.list.mockReset().mockResolvedValue({ items: presets })
  mocks.resolve
    .mockReset()
    .mockImplementation(
      async ({ baseUrl, presetId }: { baseUrl: string; presetId: string }) => {
        const preset = presets.find((p) => p.id === presetId)
        if (!preset) throw new Error('Unknown fixture preset')
        return {
          provider: preset.provider,
          endpointConfig: {
            chat: { url: `${baseUrl}/chat/completions`, auth: 'bearer' },
          },
        }
      }
    )
})
const clients: QueryClient[] = []
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
})
function setup() {
  const onCreated = vi.fn()
  const onClose = vi.fn()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <UpstreamCreateSheet onCreated={onCreated} onClose={onClose} />
    </QueryClientProvider>
  )
  return { onCreated, onClose }
}
const label = (key: string) => String(i18n.t(key))
async function selectDeepSeek() {
  fireEvent.click(
    await screen.findByRole('button', {
      name: i18n.t('channels.create.usePreset', { name: 'DeepSeek / OpenAI' }),
    })
  )
}
function field(key: string) {
  return screen.getByLabelText(label(key))
}
function change(key: string, value: string) {
  fireEvent.change(field(key), { target: { value } })
}
function submit() {
  fireEvent.click(
    screen.getByRole('button', { name: label('channels.create.submit') })
  )
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

describe('UpstreamCreateSheet', () => {
  it('prioritizes New API, domestic platforms and Coding Plan while searching all presets', async () => {
    setup()
    const region = screen.getByRole('region', {
      name: label('channels.create.preset'),
    })
    await screen.findByText('DeepSeek')
    const options = within(region).getAllByRole('button', { name: /^Use / })
    expect(options[0]).toHaveAccessibleName(
      i18n.t('channels.create.usePreset', { name: 'New API' })
    )
    expect(options).toHaveLength(3)
    expect(
      screen.queryByRole('button', {
        name: i18n.t('channels.create.usePreset', { name: 'OpenAI API' }),
      })
    ).not.toBeInTheDocument()
    change('channels.create.search', 'api.openai.com')
    expect(
      await screen.findByRole('button', {
        name: i18n.t('channels.create.usePreset', { name: 'OpenAI API' }),
      })
    ).toBeVisible()
    change('channels.create.search', '')
    fireEvent.click(
      screen.getByRole('tab', { name: label('channels.create.groups.coding') })
    )
    expect(
      within(region).getAllByRole('button', { name: /^Use / })
    ).toHaveLength(1)
  })

  it('creates with an explicit generic provider contract and no credential, then reports its id', async () => {
    const endpointConfig = {
      chat: {
        url: 'https://api.deepseek.com/v1/chat/completions',
        auth: 'bearer',
        profile: 'deepseek',
      },
    } satisfies ImportedEndpointConfig
    mocks.resolve.mockResolvedValue({ provider: 'deepseek', endpointConfig })
    const { onCreated, onClose } = setup()
    await selectDeepSeek()
    await screen.findByDisplayValue(endpointConfig.chat.url)
    submit()
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(42))
    expect(mocks.create.mock.calls[0][0]).toEqual({
      name: 'DeepSeek',
      provider: 'deepseek',
      baseUrl: 'https://api.deepseek.com/v1',
      endpointConfig,
      enabled: false,
      useSystemProxy: false,
      dialect: 'generic',
    })
    expect(onClose).not.toHaveBeenCalled()
    expect(
      screen.queryByText(label('settings.common.unsavedTitle'))
    ).not.toBeInTheDocument()
  })

  it('uses protocol pills rather than provider brands and collapses selection to reveal the form', async () => {
    setup()
    const deepseek = await screen.findByRole('button', {
      name: i18n.t('channels.create.usePreset', { name: 'DeepSeek / OpenAI' }),
    })
    expect(within(deepseek).getByText('Chat')).toBeVisible()
    expect(within(deepseek).queryByText('OpenAI')).not.toBeInTheDocument()
    expect(within(deepseek).queryByText('Responses')).not.toBeInTheDocument()
    const newapi = screen.getByRole('button', {
      name: i18n.t('channels.create.usePreset', { name: 'New API' }),
    })
    expect(within(newapi).getAllByText('New API')).toHaveLength(1)
    for (const protocol of ['Chat', 'Responses', 'Messages']) {
      expect(within(newapi).getByText(protocol)).toBeVisible()
    }
    const coding = screen.getByRole('button', {
      name: i18n.t('channels.create.usePreset', {
        name: 'Aliyun CodingPlan / Claude',
      }),
    })
    expect(within(coding).getByText('Messages')).toBeVisible()
    expect(within(coding).queryByText('Claude')).not.toBeInTheDocument()
    fireEvent.click(deepseek)
    await screen.findByDisplayValue(
      'https://api.deepseek.com/v1/chat/completions'
    )
    expect(
      screen.queryByRole('textbox', { name: label('channels.create.search') })
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
    expect(field('channels.upstream.name')).toBeVisible()
    expect(field('channels.upstream.baseUrl')).toBeVisible()
    expect(
      screen.getByRole('button', { name: label('channels.create.change') })
    ).toBeVisible()
    expect(screen.getByText(label('channels.create.description'))).toHaveClass(
      'sr-only'
    )
  })

  it('keeps the draft when changing or reselecting the platform and allows explicit enabling', async () => {
    setup()
    await selectDeepSeek()
    await screen.findByDisplayValue(
      'https://api.deepseek.com/v1/chat/completions'
    )
    change('channels.upstream.name', 'My relay')
    change('channels.upstream.baseUrl', 'https://relay.example.com/v1')
    fireEvent.blur(field('channels.upstream.baseUrl'))
    await screen.findByDisplayValue(
      'https://relay.example.com/v1/chat/completions'
    )
    expect(
      screen.getByRole('switch', { name: label('channels.create.enabled') })
    ).not.toBeChecked()
    fireEvent.click(
      screen.getByRole('switch', { name: label('channels.create.enabled') })
    )
    fireEvent.click(
      screen.getByRole('button', { name: label('channels.create.change') })
    )
    expect(field('channels.upstream.name')).toHaveValue('My relay')
    expect(field('channels.upstream.baseUrl')).toHaveValue(
      'https://relay.example.com/v1'
    )
    change('channels.create.search', 'DeepSeek')
    await selectDeepSeek()
    expect(field('channels.upstream.name')).toHaveValue('My relay')
    expect(field('channels.upstream.baseUrl')).toHaveValue(
      'https://relay.example.com/v1'
    )
    expect(
      screen.getByDisplayValue('https://relay.example.com/v1/chat/completions')
    ).toBeVisible()
    submit()
    await waitFor(() =>
      expect(mocks.create.mock.calls[0]?.[0]).toEqual(
        expect.objectContaining({
          name: 'My relay',
          baseUrl: 'https://relay.example.com/v1',
          enabled: true,
        })
      )
    )
  })

  it('starts New API with an empty host and resolves only the supplied instance address', async () => {
    setup()
    fireEvent.click(
      await screen.findByRole('button', {
        name: i18n.t('channels.create.usePreset', { name: 'New API' }),
      })
    )
    expect(field('channels.upstream.baseUrl')).toHaveValue('')
    expect(mocks.resolve).not.toHaveBeenCalled()
    change('channels.upstream.baseUrl', 'https://newapi.example.com/v1')
    fireEvent.blur(field('channels.upstream.baseUrl'))
    await screen.findByDisplayValue(
      'https://newapi.example.com/v1/chat/completions'
    )
    submit()
    await waitFor(() =>
      expect(mocks.create.mock.calls[0]?.[0]).toEqual(
        expect.objectContaining({
          provider: 'new-api',
          baseUrl: 'https://newapi.example.com/v1',
          dialect: 'generic',
        })
      )
    )
    expect(mocks.resolve).toHaveBeenCalledWith({
      presetId: 'new-api-connection',
      baseUrl: 'https://newapi.example.com/v1',
    })
  })

  it('prevents duplicate creation and closing while the request is pending', async () => {
    const pending = deferred<{ id: number }>()
    mocks.create.mockReturnValue(pending.promise)
    const { onCreated, onClose } = setup()
    await selectDeepSeek()
    await screen.findByDisplayValue(
      'https://api.deepseek.com/v1/chat/completions'
    )
    submit()
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: label('channels.create.submit') })
      ).toBeDisabled()
    )
    expect(
      screen.getByRole('button', { name: label('common.cancel') })
    ).toBeDisabled()
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()
    await act(async () => {
      pending.resolve({ id: 50 })
    })
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(50))
    expect(mocks.create).toHaveBeenCalledTimes(1)
  })

  it('clears old endpoints during URL editing, resolves on blur and preserves the returned endpoint fields', async () => {
    setup()
    await selectDeepSeek()
    await screen.findByDisplayValue(
      'https://api.deepseek.com/v1/chat/completions'
    )
    const endpointConfig = {
      gemini: {
        url: 'https://relay.example.com/google/v1beta/models',
        auth: 'x-goog-api-key',
        modelPath: true,
      },
    } satisfies ImportedEndpointConfig
    mocks.resolve.mockResolvedValue({ provider: 'deepseek', endpointConfig })
    change('channels.upstream.baseUrl', 'https://relay.example.com')
    change('channels.upstream.baseUrl', 'https://relay.example.com/google')
    expect(mocks.resolve).toHaveBeenCalledTimes(1)
    expect(
      screen.queryByDisplayValue('https://api.deepseek.com/v1/chat/completions')
    ).not.toBeInTheDocument()
    fireEvent.blur(field('channels.upstream.baseUrl'))
    await screen.findByDisplayValue(endpointConfig.gemini.url)
    expect(mocks.resolve).toHaveBeenCalledTimes(2)
    submit()
    await waitFor(() =>
      expect(mocks.create.mock.calls[0]?.[0]).toEqual(
        expect.objectContaining({
          baseUrl: 'https://relay.example.com/google',
          endpointConfig,
        })
      )
    )
  })

  it('ignores a stale resolve response that arrives after the new host has resolved', async () => {
    const old = deferred<ResolvedUpstreamPreset>()
    mocks.resolve.mockImplementationOnce(() => old.promise)
    setup()
    await selectDeepSeek()
    change('channels.upstream.baseUrl', 'https://new.example.com/v1')
    fireEvent.blur(field('channels.upstream.baseUrl'))
    await screen.findByDisplayValue(
      'https://new.example.com/v1/chat/completions'
    )
    await act(async () => {
      old.resolve({
        provider: 'deepseek',
        endpointConfig: {
          chat: {
            url: 'https://api.deepseek.com/v1/chat/completions',
            auth: 'bearer',
          },
        },
      })
    })
    expect(
      screen.queryByDisplayValue('https://api.deepseek.com/v1/chat/completions')
    ).not.toBeInTheDocument()
    submit()
    await waitFor(() =>
      expect(mocks.create.mock.calls[0]?.[0]).toEqual(
        expect.objectContaining({
          baseUrl: 'https://new.example.com/v1',
          endpointConfig: {
            chat: {
              url: 'https://new.example.com/v1/chat/completions',
              auth: 'bearer',
            },
          },
        })
      )
    )
  })

  it('reselects the same preset without losing its endpoints', async () => {
    setup()
    await selectDeepSeek()
    await screen.findByDisplayValue(
      'https://api.deepseek.com/v1/chat/completions'
    )
    fireEvent.click(
      screen.getByRole('button', { name: label('channels.create.change') })
    )
    await selectDeepSeek()
    await screen.findByDisplayValue(
      'https://api.deepseek.com/v1/chat/completions'
    )
    submit()
    await waitFor(() => expect(mocks.create).toHaveBeenCalledTimes(1))
  })

  it('requires a valid URL and a nonempty endpoint contract for manual creation', async () => {
    setup()
    fireEvent.click(
      screen.getByRole('button', { name: label('channels.create.custom') })
    )
    change('channels.upstream.name', 'Custom relay')
    change('channels.upstream.baseUrl', 'https://relay.example.com')
    submit()
    expect(
      await screen.findByText(label('channels.create.endpointsRequired'))
    ).toBeVisible()
    expect(mocks.create).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Chat' }))
    change(
      'channels.upstream.endpointUrl',
      'https://relay.example.com/v1/chat/completions'
    )
    change('channels.upstream.baseUrl', 'ftp://relay.example.com')
    submit()
    expect(
      await screen.findByText(label('channels.upstream.invalidUrl'))
    ).toBeVisible()
    expect(mocks.create).not.toHaveBeenCalled()
    change('channels.upstream.baseUrl', 'https://relay.example.com')
    fireEvent.click(screen.getByRole('checkbox', { name: 'Chat' }))
    change(
      'channels.upstream.endpointUrl',
      'https://relay.example.com/v1/chat/completions'
    )
    submit()
    await waitFor(() =>
      expect(mocks.create.mock.calls[0]?.[0]).toEqual(
        expect.objectContaining({
          provider: 'openai_compatible',
          dialect: 'generic',
        })
      )
    )
  })

  it('requires resolving the changed URL even if an endpoint is manually added afterward', async () => {
    setup()
    await selectDeepSeek()
    await screen.findByDisplayValue(
      'https://api.deepseek.com/v1/chat/completions'
    )
    change('channels.upstream.baseUrl', 'https://new.example.com')
    fireEvent.click(screen.getByRole('checkbox', { name: 'Chat' }))
    change(
      'channels.upstream.endpointUrl',
      'https://new.example.com/v1/chat/completions'
    )
    submit()
    expect(
      await screen.findByText(label('channels.create.resolveRequired'))
    ).toBeVisible()
    expect(mocks.create).not.toHaveBeenCalled()
  })

  it('keeps draft input after create failure and permits retry', async () => {
    mocks.create.mockRejectedValueOnce(new Error('offline'))
    const { onCreated } = setup()
    await selectDeepSeek()
    await screen.findByDisplayValue(
      'https://api.deepseek.com/v1/chat/completions'
    )
    change('channels.upstream.name', 'My relay')
    submit()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      label('channels.create.failed')
    )
    expect(field('channels.upstream.name')).toHaveValue('My relay')
    submit()
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(42))
  })

  it('clears failed resolution and lets the user retry without sending old endpoints', async () => {
    setup()
    await selectDeepSeek()
    await screen.findByDisplayValue(
      'https://api.deepseek.com/v1/chat/completions'
    )
    mocks.resolve.mockRejectedValueOnce(new Error('offline'))
    change('channels.upstream.baseUrl', 'https://new.example.com/v1')
    fireEvent.blur(field('channels.upstream.baseUrl'))
    expect(
      await screen.findByText(label('channels.create.resolveFailed'))
    ).toBeVisible()
    expect(
      screen.queryByDisplayValue('https://api.deepseek.com/v1/chat/completions')
    ).not.toBeInTheDocument()
    submit()
    expect(
      await screen.findByText(label('channels.create.endpointsRequired'))
    ).toBeVisible()
    expect(mocks.create).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: label('channels.create.resolve') })
    )
    await screen.findByDisplayValue(
      'https://new.example.com/v1/chat/completions'
    )
    submit()
    await waitFor(() => expect(mocks.create).toHaveBeenCalledTimes(1))
  })

  it('asks before discarding a dirty draft', async () => {
    const { onClose } = setup()
    await selectDeepSeek()
    await screen.findByDisplayValue(
      'https://api.deepseek.com/v1/chat/completions'
    )
    fireEvent.click(
      screen.getByRole('button', { name: label('common.cancel') })
    )
    expect(
      await screen.findByText(label('settings.common.unsavedTitle'))
    ).toBeVisible()
    expect(onClose).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', {
        name: label('settings.common.discardChanges'),
      })
    )
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
  })
})
