import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import type { AxiosAdapter } from 'axios'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import i18n from '@/i18n/config'
import { apiClient } from '@/lib/http-client'

import type { PersonalKey } from '../api'
import { PortalOnboarding } from '../components/portal-onboarding'

const original = apiClient.defaults.adapter
let client: QueryClient
let failed = false
let keyOverrides: Partial<PersonalKey> = {}

const modelPayload = {
  items: [
    {
      id: 'public-model',
      routeId: 1,
      protocols: ['responses'],
      referencePrice: null,
    },
  ],
  quota: {
    unit: 'token_credits',
    tokensPerUnit: 1000000,
    groupMultiplier: 1,
  },
}

beforeEach(async () => {
  await i18n.changeLanguage('en')
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  failed = false
  keyOverrides = {}
  const adapter: AxiosAdapter = async (config) => {
    if (failed) throw new Error('synthetic read failure')
    const url = config.url ?? ''
    if (url.endsWith('/models')) {
      return {
        data: modelPayload,
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    if (url.endsWith('/keys')) {
      return {
        data: {
          items: [
            {
              id: 1,
              name: 'old-key',
              keyMasked: 'mk-…old',
              enabled: false,
              expiresAt: null,
              supportedModels: [],
              maxCost: null,
              maxRequests: null,
              usedCost: 0,
              usedRequests: 0,
              createdAt: '2026-01-01T00:00:00Z',
              ...keyOverrides,
            },
          ],
        },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    return {
      data: {
        items: [
          {
            id: 'request-1',
            modelRequested: 'public-model',
            terminalStatus: 'completed',
            latencyMs: 10,
            startedAt: '2026-01-01T00:00:00Z',
            endedAt: '2026-01-01T00:00:01Z',
            attemptCount: 1,
          },
        ],
        total: 1,
        page: 1,
        pageSize: 20,
      },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }
  apiClient.defaults.adapter = adapter
})

afterEach(() => {
  cleanup()
  client.clear()
  apiClient.defaults.adapter = original
})

async function mount() {
  const root = createRootRoute()
  const route = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => <PortalOnboarding userId={1} />,
  })
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('fact-driven onboarding', () => {
  it.each([
    {
      name: 'expired',
      value: { enabled: true, expiresAt: '2020-01-01T00:00:00Z' },
    },
    { name: 'invalid expiry', value: { enabled: true, expiresAt: 'invalid' } },
    {
      name: 'request cap reached',
      value: { enabled: true, maxRequests: 2, usedRequests: 2 },
    },
    {
      name: 'cost cap reached',
      value: { enabled: true, maxCost: 1, usedCost: 1 },
    },
  ])('does not mark a $name Key as usable', async ({ value }) => {
    keyOverrides = value
    await mount()
    expect(
      await screen.findByText(
        i18n.t('userPortal.onboarding.steps.key.unavailable')
      )
    ).toBeVisible()
    expect(
      screen.queryByText(i18n.t('userPortal.onboarding.steps.key.complete'))
    ).not.toBeInTheDocument()
  })

  it('recognizes a valid Key with remaining request capacity', async () => {
    keyOverrides = { enabled: true, maxRequests: 2, usedRequests: 1 }
    await mount()
    expect(
      await screen.findByText(
        i18n.t('userPortal.onboarding.steps.key.complete')
      )
    ).toBeVisible()
  })

  it('separates an unusable existing key from a completed gateway request', async () => {
    await mount()
    expect(
      await screen.findByText('A model is available to your current group')
    ).toBeVisible()
    expect(
      await screen.findByText(
        'Existing Keys are disabled, expired or at a configured limit. Create a suitable Key or ask your administrator for help.'
      )
    ).toBeVisible()
    expect(
      screen.getByText(
        'The recent records include a completed gateway request. Confirm the returned text and tool results in your client.'
      )
    ).toBeVisible()
    expect(
      screen.getByText(/A completed status means the gateway completed/)
    ).toBeVisible()
  })

  it('does not claim completion when a fact query fails and offers retries', async () => {
    failed = true
    await mount()
    expect(
      await screen.findByText(
        'The model list could not be read, so this step is unknown. Retry before treating access as unavailable.'
      )
    ).toBeVisible()
    expect(screen.getAllByRole('button', { name: 'Retry' })).toHaveLength(4)
    expect(
      screen.queryByText(
        'The recent records include a completed gateway request. Confirm the returned text and tool results in your client.'
      )
    ).not.toBeInTheDocument()
  })
})
