// End-to-end behavior test for the comparison row re-run loop (Wave 11
// feedback loops): after a batch comparison settles, each row's re-run
// button re-probes that channel with the comparison's ORIGINAL payload via
// the existing batch machinery (no new API), failed rows included. The
// button carries a channel-specific aria-label and a per-row pending state
// (disabled + Spinner) while the probe is in flight. Mocks stop at the
// network boundary (api.testChatSync) and the form/viewer seams, so the
// page wiring, batch settling, merge, and re-sort stay real.
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
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { ChannelRow } from '@/features/channels'

import { ModelTesterPage } from '../components/model-tester-page'
import type { TesterFormValues } from '../lib/tester-schema'

type SubmitFn = (values: TesterFormValues) => void | Promise<void>

const testState = vi.hoisted(() => ({
  testChatSync: vi.fn(),
  submit: null as SubmitFn | null,
  formIsRunning: false,
}))

vi.mock('@tanstack/react-router', () => ({
  useSearch: () => ({}),
  Link: ({ children }: { children?: ReactNode }) => <a>{children}</a>,
}))

vi.mock('@/features/channels', () => ({
  useChannels: () => ({ data: channelsFixture }),
}))

vi.mock('@/lib/api', () => ({
  api: { testChatSync: testState.testChatSync },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
    message: vi.fn(),
  },
}))

vi.mock('../components/test-form', () => ({
  TestForm: ({
    isRunning,
    onSubmit,
  }: {
    isRunning: boolean
    onSubmit: SubmitFn
  }) => {
    testState.submit = onSubmit
    testState.formIsRunning = isRunning
    return (
      <button type='button' onClick={() => void onSubmit(comparisonValues)}>
        run comparison
      </button>
    )
  },
}))

vi.mock('../components/test-response-viewer', () => ({
  TestResponseViewer: () => null,
}))

vi.mock('@/components/common/confirm-dialog', () => ({
  ConfirmDialog: () => null,
}))

const channelsFixture: ChannelRow[] = [
  {
    id: 1,
    routeId: 10,
    name: 'Fast channel',
    site: { id: 20, name: 'Primary site' },
    type: 'account',
    status: 'enabled',
    models: 'gpt-4o',
    priority: 0,
    weight: 10,
    responseMs: null,
    cooldownUntil: null,
    cooldownReasonCode: null,
    cooldownReason: null,
    cooldownReasonAt: null,
    enabled: true,
    manualOverride: false,
  },
  {
    id: 2,
    routeId: 11,
    name: 'Broken channel',
    site: { id: 21, name: 'Backup site' },
    type: 'account',
    status: 'enabled',
    models: 'gpt-4o',
    priority: 0,
    weight: 10,
    responseMs: null,
    cooldownUntil: null,
    cooldownReasonCode: null,
    cooldownReason: null,
    cooldownReasonAt: null,
    enabled: true,
    manualOverride: false,
  },
]

const comparisonValues: TesterFormValues = {
  model: 'gpt-4o-mini',
  compareChannels: true,
  channelIds: [1, 2],
  systemPrompt: '',
  prompt: 'hello comparison',
  targetFormat: 'openai',
  temperature: 0,
  topP: 1,
  maxTokens: 0,
}

function successEnvelope(latencyMs: number): Response {
  return new Response(
    JSON.stringify({
      success: true,
      statusCode: 200,
      latencyMs,
      truncatedBody: JSON.stringify({
        choices: [{ message: { content: 'ok' }, finish_reason: 'stop' }],
      }),
      error: null,
    }),
    { status: 200 }
  )
}

function failureEnvelope(): Response {
  return new Response(
    JSON.stringify({
      success: false,
      statusCode: 500,
      latencyMs: 30,
      truncatedBody: '{}',
      error: 'upstream down',
    }),
    { status: 200 }
  )
}

/** Channel 1 succeeds, channel 2 fails — like a real mixed batch. */
function routeByChannel(payload: { channelId?: number }): Promise<Response> {
  return Promise.resolve(
    payload.channelId === 2 ? failureEnvelope() : successEnvelope(120)
  )
}

beforeEach(() => {
  testState.testChatSync.mockReset()
  testState.submit = null
  testState.formIsRunning = false
})

afterEach(() => cleanup())

function renderPage() {
  // The page hosts a real useTestModel mutation, which needs a provider.
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ModelTesterPage />
    </QueryClientProvider>
  )
}

async function runComparisonToSettled() {
  testState.testChatSync.mockImplementation(routeByChannel)
  renderPage()
  fireEvent.click(screen.getByRole('button', { name: 'run comparison' }))
  await waitFor(() => {
    expect(screen.getByText('1 succeeded / 1 failed')).toBeInTheDocument()
  })
}

describe('ModelTesterPage comparison row re-run', () => {
  it('re-runs a failed row with the original payload and merges the fresh result', async () => {
    await runComparisonToSettled()

    // The failed row is re-runnable.
    const rerunButton = screen.getByRole('button', {
      name: 'Re-run probe for Broken channel',
    })
    expect(rerunButton).toBeEnabled()

    // The retry now succeeds upstream.
    testState.testChatSync.mockImplementation(() =>
      Promise.resolve(successEnvelope(30))
    )
    fireEvent.click(rerunButton)

    await waitFor(() => {
      expect(screen.getByText('2 succeeded / 0 failed')).toBeInTheDocument()
    })

    // Two batch probes + one row re-run, and the re-run reused the
    // comparison's payload with the row's channelId forced in.
    expect(testState.testChatSync).toHaveBeenCalledTimes(3)
    const lastPayload = testState.testChatSync.mock.calls[2][0] as {
      channelId?: number
      messages: Array<{ role: string; content: string }>
    }
    expect(lastPayload.channelId).toBe(2)
    expect(lastPayload.messages).toContainEqual({
      role: 'user',
      content: 'hello comparison',
    })
    // The failed row's error text is gone after the merge.
    expect(screen.queryByText('upstream down')).not.toBeInTheDocument()
  })

  it('shows the per-row pending state while the re-run probe is in flight', async () => {
    await runComparisonToSettled()

    let resolveRerun: ((response: Response) => void) | undefined
    testState.testChatSync.mockReturnValueOnce(
      new Promise<Response>((resolve) => {
        resolveRerun = resolve
      })
    )

    fireEvent.click(
      screen.getByRole('button', {
        name: 'Re-run probe for Broken channel',
      })
    )

    // That row's button flips to disabled + Spinner; the page-level running
    // state reaches the form; other rows stay out of the pending set.
    await waitFor(() => {
      const pendingButton = screen.getByRole('button', {
        name: 'Re-run probe for Broken channel',
      })
      expect(pendingButton).toBeDisabled()
      expect(within(pendingButton).getByRole('status')).toBeInTheDocument()
    })
    expect(
      screen.getByRole('button', { name: 'Re-run probe for Fast channel' })
    ).toBeEnabled()
    expect(testState.formIsRunning).toBe(true)

    resolveRerun?.(successEnvelope(30))
    await waitFor(() => {
      expect(screen.getByText('2 succeeded / 0 failed')).toBeInTheDocument()
    })
    expect(
      screen.getByRole('button', {
        name: 'Re-run probe for Broken channel',
      })
    ).toBeEnabled()
  })

  it('keeps successful rows re-runnable too', async () => {
    await runComparisonToSettled()

    testState.testChatSync.mockImplementation(() =>
      Promise.resolve(successEnvelope(99))
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Re-run probe for Fast channel' })
    )

    // The re-run replaces the row in place (fresh latency) without touching
    // the other row's outcome.
    await waitFor(() => {
      expect(screen.getByText('99 ms')).toBeInTheDocument()
    })
    expect(screen.getByText('1 succeeded / 1 failed')).toBeInTheDocument()
    const lastPayload = testState.testChatSync.mock.calls.at(-1)?.[0] as {
      channelId?: number
    }
    expect(lastPayload.channelId).toBe(1)
  })
})
