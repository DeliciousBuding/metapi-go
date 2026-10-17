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

import { CredentialExportDialog } from '../credential-export-dialog'

vi.mock('@/lib/api', () => ({ api: { getDownstreamKeyExport: vi.fn() } }))
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
  it('requests credentials only after explicit unlock and puts verification before client exports', async () => {
    renderDialog()
    expect(getExport).not.toHaveBeenCalled()
    unlock()
    const verify = await screen.findByText('Verify the connection')
    const client = screen.getByRole('button', { name: 'Cherry Studio' })
    expect(
      verify.compareDocumentPosition(client) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
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
