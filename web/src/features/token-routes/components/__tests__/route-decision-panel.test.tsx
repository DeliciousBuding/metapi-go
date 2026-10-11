import '@testing-library/jest-dom/vitest'
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
import { api } from '@/lib/api'

import type { RouteDecision, RouteSummaryRow } from '../../types'
import { RouteDecisionPanel } from '../route-decision-panel'

vi.mock('@/lib/api', () => ({ api: { getRouteDecision: vi.fn() } }))
const getDecision = vi.mocked(api.getRouteDecision)
const originalScrollIntoView = HTMLElement.prototype.scrollIntoView
const route: RouteSummaryRow = {
  id: 1,
  modelPattern: 'coding',
  displayName: null,
  displayIcon: null,
  modelMapping: null,
  enabled: true,
  channelCount: 1,
  enabledChannelCount: 1,
  siteNames: [],
  decisionSnapshot: null,
  decisionRefreshedAt: '2026-10-11T10:00:00Z',
}

function decision(overrides: Partial<RouteDecision> = {}): RouteDecision {
  return {
    mode: 'adminPreview',
    requestedModel: 'coding',
    actualModel: 'coding',
    matched: true,
    routeId: 1,
    modelPattern: 'coding',
    checkedAt: '2026-10-11T10:10:00Z',
    requiredUpstreamProtocol: 2,
    allowUpstreamProtocolConversion: true,
    routingStrategy: 'weighted',
    candidates: [
      {
        channelId: -1,
        accountId: 0,
        username: '',
        label: 'Primary · Developer key',
        candidateRef: {
          kind: 'direct',
          id: 31,
          channelId: 11,
          grantId: 17,
          groupId: 21,
          credentialId: 13,
        },
        actualModel: 'provider-model',
        eligibility: 'eligible',
        blockers: [],
        probability: 0,
        probabilityEvaluated: false,
      },
    ],
    ...overrides,
  }
}

function renderPanel(
  snapshot: RouteDecision | null = null,
  modelPattern = 'coding'
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <RouteDecisionPanel
        route={{ ...route, modelPattern }}
        snapshot={snapshot}
      />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  getDecision.mockReset()
  HTMLElement.prototype.scrollIntoView = vi.fn()
})
afterEach(() => {
  cleanup()
  HTMLElement.prototype.scrollIntoView = originalScrollIntoView
})

describe('current routing preview', () => {
  it('checks only on explicit submission and displays typed direct identity, not a winner or 0%', async () => {
    getDecision.mockResolvedValue({ success: true, decision: decision() })
    renderPanel()
    expect(screen.getByText(/No upstream request is sent/)).toBeInTheDocument()
    expect(
      screen.queryByText('Primary · Developer key')
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Check routing' }))
    expect(
      await screen.findByText('Primary · Developer key')
    ).toBeInTheDocument()
    expect(getDecision).toHaveBeenCalledWith({
      model: 'coding',
      endpoint: '/v1/chat/completions',
    })
    expect(screen.getByText('Direct group member · #31')).toBeInTheDocument()
    expect(
      screen.getByText(/Upstream model: provider-model/)
    ).toBeInTheDocument()
    expect(screen.getByText('No candidate-filter blocker')).toBeInTheDocument()
    expect(screen.queryByText(/account-0|0\.0%/)).not.toBeInTheDocument()
  })

  it('keeps checked context until another explicit check, and shows native count_tokens authorization failure', async () => {
    getDecision.mockResolvedValueOnce({ success: true, decision: decision() })
    getDecision.mockResolvedValueOnce({
      success: true,
      decision: decision({
        requiredUpstreamProtocol: 8,
        allowUpstreamProtocolConversion: false,
        candidates: [
          {
            ...decision().candidates?.[0],
            eligibility: 'blocked',
            blockers: [
              {
                code: 'protocol_not_authorized',
                message: 'legacy machine message',
              },
            ],
          },
        ],
      }),
    })
    renderPanel()
    fireEvent.click(screen.getByRole('button', { name: 'Check routing' }))
    await screen.findByText('No candidate-filter blocker')
    fireEvent.click(
      screen.getByRole('combobox', { name: 'Client API / protocol' })
    )
    const option = await screen.findByRole('option', {
      name: 'Anthropic count_tokens',
    })
    fireEvent.pointerDown(option)
    fireEvent.click(option)
    await waitFor(() =>
      expect(
        screen.getByRole('combobox', { name: 'Client API / protocol' })
      ).toHaveTextContent('Anthropic count_tokens')
    )
    expect(screen.getByText('No candidate-filter blocker')).toBeInTheDocument()
    expect(
      screen.getByText('Checked input: coding · /v1/chat/completions')
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Check routing' }))
    expect(
      await screen.findByText('Blocked by candidate filter')
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'No configured endpoint is authorized for this request protocol.'
      )
    ).toBeInTheDocument()
    expect(getDecision).toHaveBeenLastCalledWith({
      model: 'coding',
      endpoint: '/v1/messages/count_tokens',
    })
  })

  it('requires a concrete model for a wildcard route', async () => {
    renderPanel(null, 'gpt-*')
    fireEvent.click(screen.getByRole('button', { name: 'Check routing' }))
    expect(await screen.findByText('Enter a client model.')).toBeInTheDocument()
    fireEvent.change(screen.getByRole('textbox', { name: 'Client model' }), {
      target: { value: 'gpt-*' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Check routing' }))
    expect(
      await screen.findByText('Use a concrete model, not a wildcard.')
    ).toBeInTheDocument()
    expect(getDecision).not.toHaveBeenCalled()
  })

  it('does not keep an earlier result visible after a recheck fails', async () => {
    getDecision.mockResolvedValueOnce({ success: true, decision: decision() })
    getDecision.mockRejectedValueOnce(new Error('offline'))
    renderPanel()
    fireEvent.click(screen.getByRole('button', { name: 'Check routing' }))
    await screen.findByText('Primary · Developer key')
    fireEvent.click(screen.getByRole('button', { name: 'Check routing' }))
    await screen.findByRole('alert')
    expect(
      screen.queryByText('Primary · Developer key')
    ).not.toBeInTheDocument()
  })

  it('marks old snapshots as incomplete without trusting fake account identity or zero probability', () => {
    renderPanel({
      requestedModel: 'saved-model',
      modelPattern: 'saved-model',
      candidates: [
        { channelId: -1, accountId: 0, username: 'account-0', probability: 0 },
      ],
    })
    const snapshot = screen.getByRole('region', { name: 'Decision snapshot' })
    expect(within(snapshot).getByTitle('saved-model')).toBeInTheDocument()
    expect(
      within(snapshot).getByText('Legacy candidate 1 · identity not recorded')
    ).toBeInTheDocument()
    expect(
      within(snapshot).getByText('Not fully evaluated')
    ).toBeInTheDocument()
    expect(
      within(snapshot).getByText(/Protocol was not specified/)
    ).toBeInTheDocument()
    expect(
      within(snapshot).queryByText(/account-0|0\.0%/)
    ).not.toBeInTheDocument()
  })

  it('shows no-match without presenting a selected channel', async () => {
    getDecision.mockResolvedValue({
      success: true,
      decision: decision({
        matched: false,
        routeId: undefined,
        modelPattern: '',
        candidates: [],
      }),
    })
    renderPanel()
    fireEvent.click(screen.getByRole('button', { name: 'Check routing' }))
    await waitFor(() =>
      expect(screen.getByText('No enabled route matched')).toBeInTheDocument()
    )
    expect(screen.getByText(/No candidate was selected/)).toBeInTheDocument()
  })
})
