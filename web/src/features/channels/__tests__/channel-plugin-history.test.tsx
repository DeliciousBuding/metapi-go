import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import i18n from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ChannelPluginHistory } from '../components/channel-plugin-history'

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  restore: vi.fn(),
  prune: vi.fn(),
}))
vi.mock('@/lib/api/channel-plugin-versions', async (original) => ({
  ...(await original<typeof import('@/lib/api/channel-plugin-versions')>()),
  channelPluginVersionsApi: mocks,
}))
vi.mock('@/lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
const version = {
  key: 'recovery',
  fingerprint: 'a'.repeat(64),
  available: false,
  taskReferences: 2,
}
beforeEach(async () => {
  vi.resetAllMocks()
  await i18n.changeLanguage('en')
  mocks.list.mockResolvedValue({ items: [version] })
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
})
function mount() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <ChannelPluginHistory />
    </QueryClientProvider>
  )
}

describe('historical task program recovery', () => {
  it('loads on demand and restores only the selected original program', async () => {
    mocks.restore.mockImplementation(async () => {
      mocks.list.mockResolvedValue({
        items: [{ ...version, available: true }],
      })
      return { ...version, restored: true }
    })
    mount()
    expect(mocks.list).not.toHaveBeenCalled()
    const expand = screen.getByRole('button', {
      name: 'Historical task programs',
    })
    expect(expand).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(expand)
    expect(await screen.findByText('Archive missing')).toBeVisible()
    expect(screen.getByText('Persistent references: 2')).toBeVisible()
    expect(screen.getByText(version.fingerprint)).toBeVisible()
    fireEvent.click(
      screen.getByRole('button', { name: 'Restore exact program' })
    )
    const dialog = await screen.findByRole('dialog')
    expect(
      within(dialog).getByText(/Restoring does not install, enable, upgrade/)
    ).toBeVisible()
    fireEvent.change(
      within(dialog).getByRole('textbox', { name: 'Original reviewed source' }),
      { target: { value: 'original-reviewed-source' } }
    )
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Restore exact program' })
    )
    await waitFor(() =>
      expect(mocks.restore).toHaveBeenCalledWith(
        version,
        'original-reviewed-source'
      )
    )
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(await screen.findByText('Archive stored')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Restore exact program' })
    ).toBeNull()
  })

  it('keeps the source after failed recovery and does not claim success', async () => {
    mocks.restore.mockRejectedValue(new Error('fingerprint mismatch'))
    mount()
    fireEvent.click(
      screen.getByRole('button', { name: 'Historical task programs' })
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Restore exact program' })
    )
    const dialog = await screen.findByRole('dialog')
    const source = within(dialog).getByRole('textbox', {
      name: 'Original reviewed source',
    })
    fireEvent.change(source, { target: { value: 'wrong-reviewed-source' } })
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Restore exact program' })
    )
    await waitFor(() => expect(mocks.restore).toHaveBeenCalledTimes(1))
    expect(source).toHaveValue('wrong-reviewed-source')
    expect(screen.queryByText('Archive stored')).toBeNull()
  })

  it('reports unavailable index as an error, not an empty archive', async () => {
    mocks.list.mockRejectedValue(new Error('reference read failed'))
    mount()
    fireEvent.click(
      screen.getByRole('button', { name: 'Historical task programs' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'reference read failed'
    )
    expect(
      screen.queryByText('No captured programs or missing task references.')
    ).toBeNull()
  })
})

const reclaimable = {
  ...version,
  available: true,
  taskReferences: 0,
  prunable: true,
}
async function settlePrune() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
}
async function openPrune() {
  fireEvent.click(
    screen.getByRole('button', { name: 'Historical task programs' })
  )
  await settlePrune()
  fireEvent.click(screen.getByRole('button', { name: 'Reclaim archive' }))
  await settlePrune()
  return screen.getByRole('alertdialog')
}
async function confirmPrune(dialog: HTMLElement) {
  fireEvent.change(within(dialog).getByRole('textbox'), {
    target: { value: 'DELETE' },
  })
  for (let second = 0; second < 3; second++) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
  }
}

describe('durable program reclamation', () => {
  it('requires a fresh countdown and exact word and blocks duplicate pending actions', async () => {
    vi.useFakeTimers()
    mocks.list.mockResolvedValue({ items: [reclaimable] })
    let resolve!: (value: unknown) => void
    mocks.prune.mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done
        })
    )
    mount()
    const dialog = await openPrune()
    expect(within(dialog).getByText(/This cannot be undone/)).toHaveTextContent(
      version.fingerprint
    )
    expect(within(dialog).getByText(/This cannot be undone/)).toHaveClass(
      'min-w-0',
      'wrap-anywhere'
    )
    expect(within(dialog).getAllByRole('button').at(-1)).toBeDisabled()
    await confirmPrune(dialog)
    const confirm = within(dialog).getByRole('button', {
      name: 'Reclaim archive',
    })
    expect(confirm).toBeEnabled()
    fireEvent.click(confirm)
    await settlePrune()
    expect(mocks.prune).toHaveBeenCalledExactlyOnceWith(reclaimable)
    expect(confirm).toBeDisabled()
    expect(
      within(dialog).getByRole('button', { name: 'Cancel' })
    ).toBeDisabled()
    fireEvent.click(confirm)
    expect(mocks.prune).toHaveBeenCalledTimes(1)
    mocks.list.mockResolvedValue({ items: [] })
    resolve({
      key: version.key,
      fingerprint: version.fingerprint,
      pruned: true,
    })
    await settlePrune()
    await settlePrune()
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(
      screen.getByRole('button', { name: 'Historical task programs' })
    ).toHaveFocus()
  })

  it('cancels without mutation and reopening does not retain confirmation', async () => {
    vi.useFakeTimers()
    mocks.list.mockResolvedValue({ items: [reclaimable] })
    mount()
    const dialog = await openPrune()
    await confirmPrune(dialog)
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await settlePrune()
    expect(mocks.prune).not.toHaveBeenCalled()
    expect(
      screen.getByRole('button', { name: 'Historical task programs' })
    ).toHaveFocus()
    fireEvent.click(screen.getByRole('button', { name: 'Reclaim archive' }))
    await settlePrune()
    const reopened = screen.getByRole('alertdialog')
    expect(within(reopened).getByRole('textbox')).toHaveValue('')
    expect(within(reopened).getAllByRole('button').at(-1)).toBeDisabled()
  })

  it('refreshes after an uncertain failure and refuses the old eligibility hint', async () => {
    vi.useFakeTimers()
    mocks.list.mockResolvedValue({ items: [reclaimable] })
    mocks.prune.mockImplementation(async () => {
      mocks.list.mockResolvedValue({
        items: [{ ...version, available: true, taskReferences: 1 }],
      })
      throw new Error('reference changed')
    })
    mount()
    const dialog = await openPrune()
    await confirmPrune(dialog)
    fireEvent.click(
      within(dialog).getByRole('button', { name: 'Reclaim archive' })
    )
    await settlePrune()
    await settlePrune()
    expect(within(dialog).getByRole('alert')).toHaveTextContent(
      'Could not confirm reclamation'
    )
    expect(
      within(dialog).getByRole('button', { name: 'Reclaim archive' })
    ).toBeDisabled()
    expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeEnabled()
    expect(mocks.prune).toHaveBeenCalledTimes(1)
  })
})
