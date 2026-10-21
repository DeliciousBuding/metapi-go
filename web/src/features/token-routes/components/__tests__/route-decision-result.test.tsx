import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import type { RouteDecision } from '../../types'
import { RouteDecisionResult } from '../route-decision-result'

const candidates: NonNullable<RouteDecision['candidates']> = [
  {
    candidateRef: { kind: 'account', id: 21, accountId: 7 },
    label: 'Alpha · primary',
    actualModel: 'model-a',
    eligibility: 'eligible',
    priority: 0,
    weight: 80,
    blockers: [],
  },
  {
    candidateRef: { kind: 'account', id: 22, accountId: 8 },
    label: 'Beta · backup',
    actualModel: 'model-b',
    eligibility: 'blocked',
    priority: 4,
    weight: 20,
    blockers: [{ code: 'source_model_mismatch', message: 'model mismatch' }],
  },
  {
    candidateRef: { kind: 'direct', id: 23 },
    label: 'Cooling by name only',
    actualModel: 'model-c',
    eligibility: 'notEvaluated',
    blockers: [],
    reasonText: 'Legacy reason retained',
  },
]
const decision: RouteDecision = {
  routeId: 9,
  matched: true,
  modelPattern: 'public-model',
  requestedModel: 'public-model',
  requiredUpstreamProtocol: 2,
  candidates,
}

function show(result = decision, onEditRoute = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <RouteDecisionResult decision={result} onEditRoute={onEditRoute} />
    </QueryClientProvider>
  )
}
afterEach(cleanup)

describe('diagnostic candidate inventory', () => {
  it('filters by evaluated state without inferring it from a candidate name', () => {
    show()
    fireEvent.click(screen.getByRole('button', { name: 'Blocked (1)' }))
    expect(screen.getByText('Beta · backup')).toBeInTheDocument()
    expect(screen.queryByText('Alpha · primary')).not.toBeInTheDocument()
    expect(screen.queryByText('Cooling by name only')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Not evaluated (1)' }))
    expect(screen.getByText('Cooling by name only')).toBeInTheDocument()
    expect(screen.getByText('Legacy reason retained')).toBeInTheDocument()
  })

  it('searches translated reasons, exact identities and models over all candidates, and resets an empty filtered view', () => {
    show()
    const search = screen.getByRole('textbox', { name: 'Find candidates' })
    fireEvent.change(search, { target: { value: 'does not match' } })
    expect(screen.getByText('Beta · backup')).toBeInTheDocument()
    expect(screen.queryByText('Alpha · primary')).not.toBeInTheDocument()
    fireEvent.change(search, { target: { value: '21' } })
    expect(screen.getByText('Alpha · primary')).toBeInTheDocument()
    fireEvent.change(search, { target: { value: 'model-c' } })
    expect(screen.getByText('Cooling by name only')).toBeInTheDocument()
    fireEvent.change(search, { target: { value: 'no-such-candidate' } })
    expect(
      screen.getByText('No candidates match these filters.')
    ).toBeInTheDocument()
    expect(
      screen.queryByText('No candidates were returned for this route.')
    ).not.toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Reset candidate filters' })
    )
    expect(screen.getAllByRole('listitem')).toHaveLength(3)
  })

  it('preserves server order and labels priority and weight as configuration, with a known route repair entry', () => {
    const onEditRoute = vi.fn()
    show(decision, onEditRoute)
    const rows = screen.getAllByRole('listitem')
    expect(within(rows[0]).getByText('Alpha · primary')).toBeInTheDocument()
    expect(
      within(rows[1]).getByText('Configured priority: 4')
    ).toBeInTheDocument()
    expect(
      within(rows[1]).getByText('Configured weight: 20')
    ).toBeInTheDocument()
    fireEvent.click(
      within(rows[1]).getByRole('button', { name: 'Edit route configuration' })
    )
    expect(onEditRoute).toHaveBeenCalledWith(9)
    expect(screen.queryByText(/0\.0%/)).not.toBeInTheDocument()
  })

  it('keeps protocol-unknown eligible candidates in the unevaluated category and hides unsafe repair targets', () => {
    show({
      ...decision,
      requiredUpstreamProtocol: undefined,
      candidates: [
        { ...candidates[0] },
        { ...candidates[1], candidateRef: { kind: 'direct', id: 22 } },
      ],
    })
    expect(
      screen.getByRole('button', { name: 'Not evaluated (1)' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Edit route configuration' })
    ).not.toBeInTheDocument()
  })

  it('distinguishes no route from a matched route with no candidates', () => {
    const view = show({
      ...decision,
      matched: false,
      modelPattern: undefined,
      candidates: [],
    })
    expect(
      screen.getByText('Check the client model or configure a matching route.')
    ).toBeInTheDocument()
    view.unmount()
    show({ ...decision, candidates: [] })
    expect(
      screen.getByText('No candidates were returned for this route.')
    ).toBeInTheDocument()
  })

  it('searches the entire long inventory before paging its visible candidates', () => {
    show({
      ...decision,
      candidates: Array.from({ length: 42 }, (_, index) => ({
        ...candidates[0],
        candidateRef: { kind: 'account', id: index + 100 },
        label: `Account ${index + 1}`,
      })),
    })
    expect(screen.getAllByRole('listitem')).toHaveLength(20)
    fireEvent.change(screen.getByRole('textbox', { name: 'Find candidates' }), {
      target: { value: 'Account 42' },
    })
    expect(screen.getByText('Account 42')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(1)
  })
})
