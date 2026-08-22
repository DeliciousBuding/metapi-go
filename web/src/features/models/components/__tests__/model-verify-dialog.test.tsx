// Behavior tests for the models batch availability probe dialog.
//
// Guards the honest-report contract of the G1 verify-batch surface: the
// dialog renders the per-target outcome table with truthful status badges
// (inconclusive is NOT success), surfaces the probe-failure message when the
// scheduler is not running, and loads verification history on demand.

import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ModelVerifyDialog } from '../model-verify-dialog'

const { mockVerifyBatch, mockVerifyHistory } = vi.hoisted(() => ({
  mockVerifyBatch: vi.fn(),
  mockVerifyHistory: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    verifyModelsBatch: mockVerifyBatch,
    getModelVerifyHistory: mockVerifyHistory,
  },
}))

beforeEach(() => {
  mockVerifyBatch.mockReset()
  mockVerifyHistory.mockReset()
})

afterEach(() => cleanup())

function renderDialog() {
  return render(<ModelVerifyDialog open onOpenChange={vi.fn()} />)
}

describe('ModelVerifyDialog', () => {
  it('runs a batch probe and shows honest per-target outcomes', async () => {
    mockVerifyBatch.mockResolvedValue({
      success: true,
      batchId: 'vb-1',
      probed: 3,
      summary: { success: 1, failure: 1, inconclusive: 1, skipped: 0 },
      items: [
        {
          model: 'gpt-4o',
          siteName: 'site-a',
          status: 'success',
          latencyMs: 120,
        },
        {
          model: 'gpt-4o-mini',
          siteName: 'site-a',
          status: 'failure',
          latencyMs: 0,
          errorText: 'upstream 502',
        },
      ],
    })

    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Run batch probe' }))

    await waitFor(() => expect(mockVerifyBatch).toHaveBeenCalledTimes(1))
    // Verdicts stay honest: failure is rendered as failure, not success.
    expect(await screen.findByText('Failed')).toBeInTheDocument()
    expect(screen.getByText('OK')).toBeInTheDocument()
    expect(screen.getByText('upstream 502')).toBeInTheDocument()
    expect(screen.getByText('probed 3')).toBeInTheDocument()
    // No success-faking: filtered target list is exactly what came back.
    expect(
      screen.queryByText('gpt-4o-mini', { selector: 'td' })
    ).toBeInTheDocument()
  })

  it('renders the probe failure message when the scheduler is not running', async () => {
    mockVerifyBatch.mockRejectedValue(
      new Error('model probe scheduler is not running (start schedulers)')
    )

    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Run batch probe' }))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(
      'Batch probe failed: model probe scheduler is not running (start schedulers)'
    )
  })

  it('loads and renders verification history on demand', async () => {
    mockVerifyBatch.mockResolvedValue({
      success: true,
      batchId: 'vb-2',
      probed: 0,
      summary: { success: 0, failure: 0, inconclusive: 0, skipped: 0 },
      items: [],
      note: 'no enabled route channels match the filter',
    })
    mockVerifyHistory.mockResolvedValue({
      items: [
        {
          id: 41,
          batchId: 'vb-1',
          model: 'claude-sonnet-4',
          siteName: 'site-b',
          status: 'inconclusive',
          latencyMs: null,
          errorText: 'dial timeout',
        },
      ],
    })

    renderDialog()
    fireEvent.click(screen.getByRole('button', { name: 'Run batch probe' }))
    await waitFor(() =>
      expect(
        screen.getByText('no enabled route channels match the filter')
      ).toBeInTheDocument()
    )

    fireEvent.click(screen.getByRole('button', { name: 'View history' }))
    await waitFor(() => expect(mockVerifyHistory).toHaveBeenCalledTimes(1))
    expect(await screen.findByText('claude-sonnet-4')).toBeInTheDocument()
    // Inconclusive stays inconclusive — nothing is dressed up as OK.
    expect(screen.getByText('Inconclusive')).toBeInTheDocument()
    expect(screen.queryByText('OK')).not.toBeInTheDocument()
  })
})
