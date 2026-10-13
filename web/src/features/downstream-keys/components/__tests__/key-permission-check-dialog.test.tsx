// Behavior tests for the key permission-check dialog: the server's verdict,
// blockers, and window numbers render as-is; the dialog never re-derives
// permission client-side.
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { KeyPermissionCheckDialog } from '../key-permission-check-dialog'

const testState = vi.hoisted(() => ({
  getRouteDecisionKeyContext: vi.fn(),
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getRouteDecisionKeyContext: testState.getRouteDecisionKeyContext,
    },
  }
})

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

function renderDialog(): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const ui: ReactElement = (
    <QueryClientProvider client={queryClient}>
      <KeyPermissionCheckDialog
        keyId={7}
        keyName='team-key'
        open={true}
        onOpenChange={() => {}}
      />
    </QueryClientProvider>
  )
  render(ui)
}

const allowResponse = {
  success: true,
  keyContext: {
    keyId: 7,
    name: 'team-key',
    admission: {
      verdict: 'allow',
      blockers: [],
      notes: ['evaluationIsReadOnly'],
      ip: { checked: false, allowed: false },
      cost: { used: 1.5, max: 10 },
    },
  },
  decision: {
    matched: true,
    matchedRoutePattern: 'gpt-*',
    candidates: [
      { eligibility: 'eligible' },
      { eligibility: 'blocked', blockers: [{ code: 'x', message: 'm' }] },
    ],
  },
}

const denyResponse = {
  success: true,
  keyContext: {
    keyId: 7,
    name: 'team-key',
    admission: {
      verdict: 'deny',
      blockers: [
        {
          dimension: 'state',
          code: 'disabled',
          message: 'API key is disabled',
        },
        {
          dimension: 'quota',
          code: 'over_quota',
          message: 'API key quota exceeded',
        },
      ],
      notes: ['evaluationIsReadOnly'],
      ip: { checked: false, allowed: false },
      quota: {
        windowStart: '2026-10-12T00:00:00Z',
        windowEnd: '2026-10-13T00:00:00Z',
        requestsUsed: 2,
        requestsMax: 2,
        tokensUsed: 0,
        costUsed: 0,
        unknownTokens: 0,
        unknownCost: 0,
        historyMissing: false,
        blocker: 'over_quota',
        message: 'API key quota exceeded',
      },
    },
  },
  decision: { matched: false, candidates: [] },
}

beforeEach(() => {
  testState.getRouteDecisionKeyContext.mockReset()
})

afterEach(() => cleanup())

describe('KeyPermissionCheckDialog', () => {
  it('runs the read-only check and renders the allow verdict with windows', async () => {
    testState.getRouteDecisionKeyContext.mockResolvedValue(allowResponse)
    renderDialog()

    fireEvent.change(screen.getByLabelText('Model'), {
      target: { value: 'gpt-4o' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Check' }))

    await waitFor(() => {
      expect(screen.getByText('Currently allowed')).toBeInTheDocument()
    })
    expect(testState.getRouteDecisionKeyContext).toHaveBeenCalledWith({
      keyId: 7,
      model: 'gpt-4o',
      endpoint: '/v1/chat/completions',
      clientIp: undefined,
    })
    expect(screen.getByText(/1.5 \/ 10/)).toBeInTheDocument()
    expect(
      screen.getByText(/Route gpt-\*: 1 eligible, 1 blocked/)
    ).toBeInTheDocument()
  })

  it('renders every current blocker with its dimension label', async () => {
    testState.getRouteDecisionKeyContext.mockResolvedValue(denyResponse)
    renderDialog()

    fireEvent.change(screen.getByLabelText('Model'), {
      target: { value: 'gpt-4o' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Check' }))

    await waitFor(() => {
      expect(screen.getByText('Currently blocked')).toBeInTheDocument()
    })
    expect(screen.getByText('API key is disabled')).toBeInTheDocument()
    expect(screen.getByText('API key quota exceeded')).toBeInTheDocument()
    expect(screen.getByText('state')).toBeInTheDocument()
    expect(screen.getByText('quota')).toBeInTheDocument()
    expect(screen.getByText(/2 \/ 2/)).toBeInTheDocument()
    expect(
      screen.getByText('No enabled route matched this model')
    ).toBeInTheDocument()
  })

  it('keeps the run button disabled without a model', () => {
    renderDialog()
    expect(screen.getByRole('button', { name: 'Check' })).toBeDisabled()
  })
})
