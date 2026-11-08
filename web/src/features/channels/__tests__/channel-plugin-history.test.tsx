import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
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

const mocks = vi.hoisted(() => ({ list: vi.fn(), restore: vi.fn() }))
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
afterEach(cleanup)
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
    expect(screen.getByText('Task references: 2')).toBeVisible()
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
