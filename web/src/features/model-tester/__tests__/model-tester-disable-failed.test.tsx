// Behavior test for the batch-test closure loop (Wave 17 P1-3): after a
// comparison settles with failed rows, the bulk "disable failed channels"
// action confirms with the operator, then PUTs /api/channels/batch with
// `enabled:false` for exactly the failed channel ids and surfaces the
// per-item truth envelope (full success vs partial failure) via toasts.
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { ChannelRow } from '@/features/channels'
import { toast } from '@/lib/toast'

import { ModelTesterPage } from '../components/model-tester-page'
import type { TesterFormValues } from '../lib/tester-schema'

type SubmitFn = (values: TesterFormValues) => void | Promise<void>

const testState = vi.hoisted(() => ({
  testChatSync: vi.fn(),
  batchUpdateChannels: vi.fn(),
  submit: null as SubmitFn | null,
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

vi.mock('@/lib/api/token-routes', () => ({
  tokenRoutesApi: { batchUpdateChannels: testState.batchUpdateChannels },
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
  TestForm: ({ onSubmit }: { isRunning: boolean; onSubmit: SubmitFn }) => {
    testState.submit = onSubmit
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

// Controllable stand-in: render an actionable dialog when open so the test
// can drive confirm/cancel without radix focus plumbing.
vi.mock('@/components/common/confirm-dialog', () => ({
  ConfirmDialog: ({
    open,
    title,
    onConfirm,
    onCancel,
  }: {
    open: boolean
    title: string
    onConfirm: () => void
    onCancel: () => void
  }) =>
    open ? (
      <div role='dialog' aria-label={title}>
        <button type='button' onClick={onConfirm}>
          confirm-action
        </button>
        <button type='button' onClick={onCancel}>
          cancel-action
        </button>
      </div>
    ) : null,
}))

const channelsFixture: ChannelRow[] = [
  {
    id: 1,
    routeId: 10,
    name: 'Fast channel',
    site: { id: 20, name: 'Primary site' },
    type: 'account',
    status: 'enabled',
    models: 'gpt-5.5',
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
    models: 'gpt-5.5',
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
  model: 'gpt-5-mini',
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

function routeByChannel(payload: { channelId?: number }): Promise<Response> {
  return Promise.resolve(
    payload.channelId === 2 ? failureEnvelope() : successEnvelope(120)
  )
}

beforeEach(() => {
  testState.testChatSync.mockReset()
  testState.batchUpdateChannels.mockReset()
  vi.mocked(toast.success).mockClear()
  vi.mocked(toast.warning).mockClear()
  vi.mocked(toast.error).mockClear()
})

afterEach(() => cleanup())

function renderPage() {
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

describe('ModelTesterPage bulk disable of failed channels', () => {
  it('disables exactly the failed channels after operator confirmation', async () => {
    testState.batchUpdateChannels.mockResolvedValue({
      success: true,
      successIds: [2],
      failedItems: [],
      channels: [],
    })
    await runComparisonToSettled()

    fireEvent.click(
      screen.getByRole('button', { name: 'Disable failed channels' })
    )

    // Operator confirmation is mandatory before any write.
    await screen.findByRole('dialog', {
      name: 'Disable failed channels?',
    })
    expect(testState.batchUpdateChannels).not.toHaveBeenCalled()

    fireEvent.click(
      await screen.findByRole('button', { name: 'confirm-action' })
    )

    await waitFor(() => {
      expect(testState.batchUpdateChannels).toHaveBeenCalledWith([
        { id: 2, enabled: false },
      ])
    })
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalled()
    })
  })

  it('surfaces per-item failures from the batch envelope', async () => {
    testState.batchUpdateChannels.mockResolvedValue({
      success: false,
      successIds: [],
      failedItems: [{ id: 2, message: 'channel not found' }],
      channels: [],
    })
    await runComparisonToSettled()

    fireEvent.click(
      screen.getByRole('button', { name: 'Disable failed channels' })
    )
    await screen.findByRole('dialog', { name: 'Disable failed channels?' })
    fireEvent.click(
      await screen.findByRole('button', { name: 'confirm-action' })
    )

    await waitFor(() => {
      expect(toast.warning).toHaveBeenCalled()
    })
  })

  it('cancelling the confirmation performs no write', async () => {
    await runComparisonToSettled()

    fireEvent.click(
      screen.getByRole('button', { name: 'Disable failed channels' })
    )
    await screen.findByRole('dialog', { name: 'Disable failed channels?' })
    fireEvent.click(
      await screen.findByRole('button', { name: 'cancel-action' })
    )

    await waitFor(() => {
      expect(
        screen.queryByRole('dialog', { name: 'Disable failed channels?' })
      ).not.toBeInTheDocument()
    })
    expect(testState.batchUpdateChannels).not.toHaveBeenCalled()
  })
})
