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
import { api } from '@/lib/api'
import { copyText } from '@/lib/clipboard'

import { CredentialExportDialog } from '../credential-export-dialog'

vi.mock('@/lib/api', () => ({ api: { getDownstreamKeyExport: vi.fn() } }))
vi.mock('@/lib/clipboard', () => ({ copyText: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children }: { to: string; children: ReactNode }) => (
    <a href={to}>{children}</a>
  ),
}))

const exportData = {
  success: true,
  formatVersion: '1',
  keyId: 1,
  keyName: 'demo-key',
  baseUrl: 'http://gateway.example.com',
  profiles: [
    {
      id: 'generic',
      label: 'Generic JSON',
      content: { apiKey: 'sk-plain-demo' },
    },
  ],
}
const target = { id: 1, name: 'demo-key', keyMasked: 'sk-***' }
const getExport = vi.mocked(api.getDownstreamKeyExport)

beforeEach(() => {
  getExport.mockReset()
  getExport.mockResolvedValue(exportData)
  vi.mocked(copyText).mockReset().mockResolvedValue(true)
})
afterEach(cleanup)

function renderDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const view = render(
    <QueryClientProvider client={client}>
      <CredentialExportDialog target={target} onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  return {
    client,
    rerender: (next: typeof target | null) =>
      view.rerender(
        <QueryClientProvider client={client}>
          <CredentialExportDialog target={next} onOpenChange={() => {}} />
        </QueryClientProvider>
      ),
  }
}

function unlock(token = 'master-token') {
  fireEvent.change(screen.getByPlaceholderText('Paste the admin token'), {
    target: { value: token },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Unlock' }))
}

describe('CredentialExportDialog', () => {
  it('keeps the copy action available without a success check when clipboard access fails', async () => {
    vi.mocked(copyText).mockResolvedValue(false)
    renderDialog()
    unlock()
    const copy = await screen.findByRole('button', {
      name: 'Copy API key',
    })
    fireEvent.click(copy)
    await waitFor(() => expect(copyText).toHaveBeenCalledWith('sk-plain-demo'))
    await waitFor(() => expect(copy.querySelector('.lucide-check')).toBeNull())
  })

  it('announces a successful key copy through the button accessible name', async () => {
    renderDialog()
    unlock()
    fireEvent.click(await screen.findByRole('button', { name: 'Copy API key' }))
    expect(
      await screen.findByRole('button', { name: 'API key copied' })
    ).toBeVisible()
  })

  it('unlocks into request verification and keeps client exports in their own tab', async () => {
    renderDialog()
    expect(getExport).not.toHaveBeenCalled()
    unlock()
    expect(await screen.findByText('Verify the connection')).toBeVisible()
    expect(screen.getByRole('tab', { name: 'Test request' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(screen.queryByRole('button', { name: 'Cherry Studio' })).toBeNull()
    fireEvent.click(screen.getByRole('tab', { name: 'Client setup' }))
    expect(
      await screen.findByRole('button', { name: 'Cherry Studio' })
    ).toBeVisible()
    expect(screen.queryByText('sk-plain-demo')).toBeNull()
    fireEvent.click(screen.getByRole('tab', { name: 'Test request' }))
    expect(screen.getByText('Verify the connection')).toBeVisible()
    expect(getExport).toHaveBeenCalledWith(1, 'all', 'master-token')
    expect(
      screen.queryByRole('link', { name: 'Send a test request' })
    ).toBeNull()
  })

  it('retries with the corrected admin token after rejection', async () => {
    getExport.mockRejectedValueOnce({
      response: { data: { reauthRequired: true } },
    })
    renderDialog()
    unlock('wrong-token')
    await screen.findByText('Token rejected. Check the value and try again.')
    unlock('correct-token')
    expect(await screen.findByText('Verify the connection')).toBeInTheDocument()
    expect(getExport.mock.calls.map((call) => call[2])).toEqual([
      'wrong-token',
      'correct-token',
    ])
  })

  it('relocks when switching keys without fetching the new credential', async () => {
    const view = renderDialog()
    unlock()
    await screen.findByText('Verify the connection')
    view.rerender({ id: 2, name: 'second-key', keyMasked: 'sk-second' })
    expect(screen.getByPlaceholderText('Paste the admin token')).toHaveValue('')
    expect(screen.queryByText('Verify the connection')).not.toBeInTheDocument()
    expect(getExport).toHaveBeenCalledTimes(1)
  })

  it('drops exported credentials on close and requires a fresh request after reopening', async () => {
    const view = renderDialog()
    unlock()
    await screen.findByText('Verify the connection')
    view.rerender(null)
    await waitFor(() => {
      expect(view.client.getQueryCache().getAll()).toHaveLength(0)
      expect(view.client.getMutationCache().getAll()).toHaveLength(0)
    })
    view.rerender(target)
    expect(screen.getByPlaceholderText('Paste the admin token')).toHaveValue('')
    expect(screen.queryByText('Verify the connection')).not.toBeInTheDocument()
    unlock()
    await waitFor(() => expect(getExport).toHaveBeenCalledTimes(2))
  })

  it('can retry a failed export without losing the submitted token', async () => {
    getExport.mockRejectedValueOnce(new Error('temporarily unavailable'))
    renderDialog()
    unlock()
    fireEvent.click(await screen.findByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Verify the connection')).toBeInTheDocument()
    expect(getExport).toHaveBeenLastCalledWith(1, 'all', 'master-token')
  })
})
