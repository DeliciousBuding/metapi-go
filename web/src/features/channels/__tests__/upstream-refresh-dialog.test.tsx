// Behavior tests for the catalog refresh diff dialog (route-decision E batch):
// preview renders the grouped diff with impact, apply sends the basis token,
// a 409 stale-basis response triggers a fresh preview instead of a dead end.

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
import type { UpstreamRefreshPreview } from '@/lib/api/upstream-catalog'

import { UpstreamRefreshDialog } from '../components/upstream-refresh-dialog'

const state = vi.hoisted(() => ({
  preview: vi.fn(),
  apply: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    refreshUpstreamCatalogPreview: state.preview,
    applyUpstreamCatalogRefresh: state.apply,
  },
}))
vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

function makePreview(
  over: Partial<UpstreamRefreshPreview> = {}
): UpstreamRefreshPreview {
  return {
    channelId: 7,
    basisToken: 'basis-1',
    discovery: { status: 'discovered' },
    upstreamCount: 3,
    added: ['m4'],
    stale: [
      { id: 51, name: 'm1', enabled: true, grantCount: 2, routeIds: [9, 10] },
    ],
    kept: ['m2'],
    offline: [],
    manualOnly: ['manual-keep'],
    manualKept: [],
    legacyNativeCount: 1,
    ...over,
  }
}

function renderDialog(onApplied = vi.fn()) {
  return {
    onApplied,
    ...render(
      <UpstreamRefreshDialog
        channelId={7}
        channelName='百炼'
        open
        onOpenChange={() => {}}
        onApplied={onApplied}
      />
    ),
  }
}

beforeEach(() => {
  state.preview.mockReset()
  state.apply.mockReset()
})

afterEach(() => {
  cleanup()
})

describe('UpstreamRefreshDialog', () => {
  it('renders the grouped diff with stale impact and the legacy note', async () => {
    state.preview.mockResolvedValue(makePreview())
    renderDialog()

    expect(await screen.findByText(/3 models/)).toBeInTheDocument()
    expect(screen.getByText('m4')).toBeInTheDocument()
    expect(screen.getByText('m1')).toBeInTheDocument()
    expect(screen.getByText(/2 grants · 2 routes affected/)).toBeInTheDocument()
    expect(screen.getByText('manual-keep')).toBeInTheDocument()
    expect(screen.getByText(/predate source attribution/)).toBeInTheDocument()
  })

  it('applies with the basis token and reports the outcome', async () => {
    state.preview.mockResolvedValue(makePreview())
    state.apply.mockResolvedValue({
      success: true,
      addedCount: 1,
      staleCount: 1,
      affectedRouteIds: [9, 10],
    })
    const onApplied = vi.fn()
    renderDialog(onApplied)

    const applyButton = await screen.findByRole('button', { name: 'Apply' })
    fireEvent.click(applyButton)

    await waitFor(() => expect(state.apply).toHaveBeenCalledWith(7, 'basis-1'))
    await waitFor(() => expect(onApplied).toHaveBeenCalled())
  })

  it('re-previews on a stale basis (409) instead of dead-ending', async () => {
    state.preview
      .mockResolvedValueOnce(makePreview({ basisToken: 'basis-1' }))
      .mockResolvedValueOnce(makePreview({ basisToken: 'basis-2' }))
    state.apply.mockRejectedValue(
      Object.assign(new Error('conflict'), { status: 409 })
    )
    renderDialog()

    const applyButton = await screen.findByRole('button', { name: 'Apply' })
    fireEvent.click(applyButton)

    await waitFor(() => expect(state.preview).toHaveBeenCalledTimes(2))
  })

  it('disables apply when nothing would change', async () => {
    state.preview.mockResolvedValue(
      makePreview({ added: [], stale: [], upstreamCount: 2 })
    )
    renderDialog()

    expect(
      await screen.findByText(/already matches upstream/)
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled()
  })

  it('shows the discovery error with a retry action', async () => {
    state.preview.mockRejectedValue(
      new Error('upstream does not serve a catalog')
    )
    renderDialog()

    expect(
      await screen.findByText('upstream does not serve a catalog')
    ).toBeInTheDocument()
    state.preview.mockResolvedValue(makePreview({ added: [], stale: [] }))
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(state.preview).toHaveBeenCalledTimes(2))
  })
})
