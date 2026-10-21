import '@testing-library/jest-dom/vitest'
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
import { api } from '@/lib/api'

import type { RouteDecision } from '../../types'
import { RouteWorkspace } from '../route-workspace'

vi.mock('@/lib/api', () => ({
  api: {
    getDownstreamApiKeys: vi.fn(),
    getRouteDecision: vi.fn(),
    getRouteDecisionKeyContext: vi.fn(),
  },
}))

const getKeys = vi.mocked(api.getDownstreamApiKeys)
const getDecision = vi.mocked(api.getRouteDecision)
const getKeyDecision = vi.mocked(api.getRouteDecisionKeyContext)

const route: RouteDecision = {
  mode: 'adminPreview',
  requestedModel: 'gpt-4o',
  actualModel: 'gpt-4o',
  matched: true,
  routeId: 12,
  modelPattern: 'gpt-4o',
  checkedAt: '2026-10-15T10:00:00Z',
  requiredUpstreamProtocol: 2,
  allowUpstreamProtocolConversion: true,
  routingStrategy: 'weighted',
  candidates: [
    {
      channelId: 42,
      accountId: 7,
      username: 'primary',
      label: 'Primary account',
      candidateRef: { kind: 'account', id: 42, accountId: 7, siteId: 3 },
      actualModel: 'gpt-4o',
      eligibility: 'eligible',
      blockers: [],
      probability: 0,
      probabilityEvaluated: false,
    },
  ],
}

const keyContextResult = {
  success: true,
  keyContext: {
    keyId: 7,
    name: 'prod',
    admission: {
      verdict: 'deny' as const,
      blockers: [
        {
          dimension: 'policy' as const,
          code: 'access_policy',
          message: 'The source key is blocked.',
        },
      ],
      notes: [],
      ip: { checked: false, allowed: false },
    },
  },
  decision: route,
}

function renderWorkspace(
  overrides: {
    model?: string
    keyId?: string
    onOpenRoute?: (id: number) => void
    onOpenKeyLogs?: (id: number) => void
    onOpenKeyEdit?: (id: number, locateField: string | null) => void
  } = {}
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  })
  return render(
    <QueryClientProvider client={client}>
      <RouteWorkspace
        filters={{
          workspaceModel: overrides.model ?? 'gpt-4o',
          workspaceEndpoint: '/v1/chat/completions',
          workspaceKeyId: overrides.keyId ?? '',
          workspaceClientIp: '',
        }}
        onFiltersChange={vi.fn()}
        onOpenRoute={overrides.onOpenRoute ?? vi.fn()}
        onOpenKeyLogs={overrides.onOpenKeyLogs ?? vi.fn()}
        onOpenKeyEdit={overrides.onOpenKeyEdit ?? vi.fn()}
      />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  getKeys.mockReset().mockResolvedValue({
    items: [{ id: 7, name: 'prod', enabled: true }],
  } as never)
  getDecision.mockReset()
  getKeyDecision.mockReset()
})

afterEach(() => cleanup())

describe('RouteWorkspace', () => {
  it('runs the administrator preview when no downstream key is selected', async () => {
    getDecision.mockResolvedValue({ success: true, decision: route } as never)

    renderWorkspace()

    expect(
      screen.getByRole('combobox', { name: 'Client endpoint' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('combobox', { name: 'Downstream key' })
    ).toBeInTheDocument()
    expect(await screen.findByText('Primary account')).toBeInTheDocument()
    expect(getDecision).toHaveBeenCalledWith({
      model: 'gpt-4o',
      endpoint: '/v1/chat/completions',
    })
    expect(
      screen.getByText(/administrator route preview only/i)
    ).toBeInTheDocument()
    expect(
      screen.queryByText(/read-only admission and candidate evaluation/i)
    ).not.toBeInTheDocument()
  })

  it('evaluates key admission and route candidates in one workspace', async () => {
    getKeyDecision.mockResolvedValue(keyContextResult as never)

    renderWorkspace({ keyId: '7' })

    expect(await screen.findByText('Currently blocked')).toBeInTheDocument()
    expect(screen.getByText(/The source key is blocked/i)).toBeInTheDocument()
    expect(screen.getByText('Primary account')).toBeInTheDocument()
    expect(getKeyDecision).toHaveBeenCalledWith({
      keyId: 7,
      model: 'gpt-4o',
      endpoint: '/v1/chat/completions',
      clientIp: undefined,
    })
    expect(getDecision).not.toHaveBeenCalled()
  })

  it('keeps the route and key log drilldowns in the same workspace', async () => {
    getKeyDecision.mockResolvedValue(keyContextResult as never)
    const onOpenRoute = vi.fn()
    const onOpenKeyLogs = vi.fn()

    renderWorkspace({ keyId: '7', onOpenRoute, onOpenKeyLogs })

    fireEvent.click(await screen.findByRole('button', { name: /Open route/i }))
    fireEvent.click(screen.getByRole('button', { name: /View key logs/i }))

    expect(onOpenRoute).toHaveBeenCalledWith(12)
    expect(onOpenKeyLogs).toHaveBeenCalledWith(7)
  })

  it('links a denied key to the matching editable admission dimension', async () => {
    getKeyDecision.mockResolvedValue(keyContextResult as never)
    const onOpenKeyEdit = vi.fn()

    renderWorkspace({ keyId: '7', onOpenKeyEdit })

    fireEvent.click(await screen.findByRole('button', { name: /Edit key/i }))

    expect(onOpenKeyEdit).toHaveBeenCalledWith(7, 'accessPolicy')
  })

  it('rechecks unchanged administrator input, hides a failed cached result and permits retry', async () => {
    getDecision
      .mockResolvedValueOnce({ success: true, decision: route } as never)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ success: true, decision: route } as never)
    renderWorkspace()
    await screen.findByText('Primary account')
    const check = screen.getByRole('button', { name: 'Check call path' })
    fireEvent.click(check)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Call-path check failed'
    )
    expect(getDecision).toHaveBeenCalledTimes(2)
    expect(screen.queryByText('Primary account')).not.toBeInTheDocument()
    fireEvent.click(check)
    expect(await screen.findByText('Primary account')).toBeInTheDocument()
    expect(getDecision).toHaveBeenCalledTimes(3)
  })

  it('rechecks only the selected key context and removes failed admission as well as candidates', async () => {
    getKeyDecision
      .mockResolvedValueOnce(keyContextResult as never)
      .mockRejectedValueOnce(new Error('offline'))
    renderWorkspace({ keyId: '7' })
    await screen.findByText('Currently blocked')
    fireEvent.click(screen.getByRole('button', { name: 'Check call path' }))
    await screen.findByRole('alert')
    expect(screen.queryByText('Currently blocked')).not.toBeInTheDocument()
    expect(screen.queryByText('Primary account')).not.toBeInTheDocument()
    expect(getKeyDecision).toHaveBeenCalledTimes(2)
    expect(getDecision).not.toHaveBeenCalled()
  })

  it('disables empty submissions without starting either disabled query', async () => {
    renderWorkspace({ model: '' })
    fireEvent.click(screen.getByRole('button', { name: 'Route workspace' }))
    expect(
      screen.getByRole('button', { name: 'Check call path' })
    ).toBeDisabled()
    await waitFor(() => expect(getKeys).toHaveBeenCalledOnce())
    expect(getDecision).not.toHaveBeenCalled()
    expect(getKeyDecision).not.toHaveBeenCalled()
  })
})
