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
import type { AxiosAdapter } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ThemeProvider } from '@/context/theme-provider'
import i18n from '@/i18n/config'
import { apiClient } from '@/lib/http-client'

import type { PersonalKey, PersonalModel } from '../api'
import { PortalKeys } from '../components/portal-keys'
import { readModelContext } from '../connection-context'

const secret = 'synthetic-personal-connection-once'
const item: PersonalKey = {
  id: 17,
  name: 'Editor',
  keyMasked: 'sk-fixture****abcd',
  enabled: true,
  expiresAt: null,
  supportedModels: [],
  maxCost: null,
  maxRequests: null,
  usedCost: 0,
  usedRequests: 0,
  createdAt: '2026-11-05T00:00:00Z',
}
const model: PersonalModel = {
  id: 'public-model',
  routeId: 1,
  protocols: ['chat/completions', 'responses'],
  referencePrice: null,
}
const modelPayload = (items: PersonalModel[]) => ({
  items,
  quota: { unit: 'token_credits', tokensPerUnit: 1000000, groupMultiplier: 1 },
})
const originalAdapter = apiClient.defaults.adapter
let client: QueryClient
let calls: Array<{ url: string; method: string; body: unknown }>
let modelsResponse: () => Promise<unknown>
let createResponse: () => Promise<unknown>

beforeEach(async () => {
  await i18n.changeLanguage('en')
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  calls = []
  modelsResponse = async () => modelPayload([model])
  createResponse = async () => ({ item, key: secret })
  const adapter: AxiosAdapter = async (config) => {
    const url = config.url ?? ''
    const method = config.method ?? 'get'
    calls.push({
      url,
      method,
      body:
        typeof config.data === 'string' ? JSON.parse(config.data) : undefined,
    })
    let data: unknown = { items: [] }
    if (url === '/api/user/models') data = await modelsResponse()
    else if (method === 'post') data = await createResponse()
    return { data, status: 200, statusText: 'OK', headers: {}, config }
  }
  apiClient.defaults.adapter = adapter
  Element.prototype.scrollIntoView = vi.fn()
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
    })),
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  apiClient.defaults.adapter = originalAdapter
  vi.restoreAllMocks()
})

function portal(search: string, userId = 1) {
  return (
    <QueryClientProvider client={client}>
      <ThemeProvider defaultTheme='light'>
        <PortalKeys
          userId={userId}
          connectionContext={readModelContext(search)}
        />
      </ThemeProvider>
    </QueryClientProvider>
  )
}

function mount(search = '?model=public-model&protocol=responses') {
  return render(portal(search))
}

async function createKey() {
  fireEvent.click(screen.getByRole('button', { name: 'Create key' }))
  fireEvent.change(await screen.findByLabelText('Key name'), {
    target: { value: 'Editor' },
  })
  fireEvent.click(
    within(screen.getByRole('dialog')).getByRole('button', {
      name: 'Create key',
    })
  )
  await screen.findByLabelText('Secret API key')
}

function assertSecretNotStored() {
  expect(
    JSON.stringify(
      client
        .getQueryCache()
        .getAll()
        .map((query) => query.state.data)
    )
  ).not.toContain(secret)
  expect(client.getMutationCache().getAll()).toHaveLength(0)
  expect(JSON.stringify(localStorage)).not.toContain(secret)
  expect(JSON.stringify(sessionStorage)).not.toContain(secret)
  expect(window.location.href).not.toContain(secret)
}

describe('personal model-to-key connection', () => {
  it('uses the selected model/protocol after creation without changing key permissions or repeating a model read', async () => {
    client.setQueryData(['userPortal', 1, 'models'], modelPayload([model]))
    mount()
    await createKey()
    const example = screen.getByLabelText('Request example')
    expect(example).toHaveTextContent('/v1/responses')
    expect(example).toHaveTextContent('"model": "public-model"')
    expect(example).toHaveTextContent('METAPI_API_KEY')
    expect(example).not.toHaveTextContent(secret)
    expect(calls.filter((call) => call.method === 'post')).toEqual([
      { url: '/api/user/keys', method: 'post', body: { name: 'Editor' } },
    ])
    expect(calls.some((call) => call.url === '/api/user/models')).toBe(false)
    expect(screen.getByLabelText('Secret API key')).toHaveAttribute(
      'type',
      'password'
    )
    assertSecretNotStored()
    fireEvent.click(
      screen.getByRole('button', { name: 'I have saved the key' })
    )
    await waitFor(() =>
      expect(screen.queryByLabelText('Secret API key')).not.toBeInTheDocument()
    )
    expect(screen.queryByLabelText('Request example')).not.toBeInTheDocument()
  })

  it('reads an uncached catalog only on explicit action and never sends a charged request', async () => {
    mount()
    await createKey()
    expect(screen.queryByLabelText('Request example')).not.toBeInTheDocument()
    expect(calls.some((call) => call.url === '/api/user/models')).toBe(false)
    fireEvent.click(
      screen.getByRole('button', {
        name: i18n.t('userPortal.connection.refreshDetails'),
      })
    )
    expect(await screen.findByLabelText('Request example')).toHaveTextContent(
      '/v1/responses'
    )
    expect(
      calls.filter((call) => call.url === '/api/user/models')
    ).toHaveLength(1)
    expect(calls.filter((call) => call.method === 'post')).toHaveLength(1)
    assertSecretNotStored()
  })

  it('does not describe the default wildcard as an additional model restriction', async () => {
    client.setQueryData(['userPortal', 1, 'models'], modelPayload([model]))
    createResponse = async () => ({
      item: { ...item, supportedModels: ['*'] },
      key: secret,
    })
    mount()
    await createKey()
    expect(screen.getByLabelText('Request example')).toBeVisible()
    expect(
      screen.queryByText(/This key additionally restricts models/)
    ).not.toBeInTheDocument()
  })

  it('removes stale examples during a refresh failure and lets the current catalog determine recovery', async () => {
    client.setQueryData(['userPortal', 1, 'models'], modelPayload([model]))
    mount()
    await createKey()
    expect(screen.getByLabelText('Request example')).toBeVisible()
    modelsResponse = async () => {
      throw new Error('synthetic catalog failure')
    }
    fireEvent.click(
      screen.getByRole('button', {
        name: i18n.t('userPortal.connection.refreshDetails'),
      })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load')
    expect(screen.queryByLabelText('Request example')).not.toBeInTheDocument()
    modelsResponse = async () =>
      modelPayload([{ ...model, protocols: ['chat/completions'] }])
    fireEvent.click(
      screen.getByRole('button', {
        name: i18n.t('userPortal.connection.refreshDetails'),
      })
    )
    await waitFor(() =>
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    )
    expect(screen.queryByLabelText('Request example')).not.toBeInTheDocument()
    expect(
      screen.getByText(
        i18n.t('userPortal.connection.protocolUnavailable', {
          model: model.id,
          protocol: 'responses',
        })
      )
    ).toBeVisible()
  })

  it('never invents a model sample for an unavailable model or malformed navigation context', async () => {
    client.setQueryData(['userPortal', 1, 'models'], modelPayload([]))
    const view = mount()
    await createKey()
    expect(screen.queryByLabelText('Request example')).not.toBeInTheDocument()
    expect(
      screen.getByText(/selected model public-model is no longer/)
    ).toBeVisible()
    view.rerender(portal('?model=public-model&protocol=unexpected'))
    await createKey()
    expect(screen.queryByLabelText('Request example')).not.toBeInTheDocument()
    expect(
      screen.getByText(i18n.t('userPortal.connection.invalidContext'))
    ).toBeVisible()
    expect(calls.some((call) => call.url === '/api/user/models')).toBe(false)
  })

  it('preserves ordinary creation without a selection and discards secrets when user or connection changes', async () => {
    const view = mount('?view=keys')
    await createKey()
    expect(screen.queryByLabelText('Request example')).not.toBeInTheDocument()
    view.rerender(portal('?model=public-model&protocol=responses'))
    expect(screen.queryByDisplayValue(secret)).not.toBeInTheDocument()
    await createKey()
    view.rerender(portal('?model=public-model&protocol=responses', 2))
    expect(screen.queryByDisplayValue(secret)).not.toBeInTheDocument()
    assertSecretNotStored()
  })

  it('discards a late one-time creation response after the selected connection changes', async () => {
    let finish!: (value: unknown) => void
    createResponse = () =>
      new Promise((resolve) => {
        finish = resolve
      })
    const view = mount()
    fireEvent.click(screen.getByRole('button', { name: 'Create key' }))
    fireEvent.change(await screen.findByLabelText('Key name'), {
      target: { value: 'Editor' },
    })
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Create key',
      })
    )
    await waitFor(() =>
      expect(calls.some((call) => call.method === 'post')).toBe(true)
    )
    view.rerender(portal('?model=another-model&protocol=messages'))
    await act(async () => finish({ item, key: secret }))
    expect(screen.queryByDisplayValue(secret)).not.toBeInTheDocument()
    assertSecretNotStored()
  })
})
