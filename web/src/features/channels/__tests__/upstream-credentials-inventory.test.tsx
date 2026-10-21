import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { UpstreamCredentials } from '../components/upstream-credentials'

const network = vi.hoisted(() => ({ credentials: vi.fn(), update: vi.fn() }))
vi.mock('@/lib/api', () => ({
  api: {
    getImportedCredentials: network.credentials,
    updateImportedCredential: network.update,
  },
}))
vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

let client: QueryClient
const dirty = vi.fn()
function mount(count = 300) {
  network.credentials.mockResolvedValue({
    items: Array.from({ length: count }, (_, index) => ({
      id: index + 1,
      name: `Team ${String(index + 1).padStart(3, '0')}`,
      enabled: index % 3 !== 0,
      kind: index === 249 ? 'oauth' : 'api_key',
      canRefresh: false,
    })),
  })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UpstreamCredentials id={4} active onDirtyChange={dirty} />
    </QueryClientProvider>
  )
}
beforeEach(() => {
  vi.clearAllMocks()
  network.update.mockResolvedValue({ success: true })
})
afterEach(() => {
  cleanup()
  client?.clear()
})

describe('credential inventory', () => {
  it('starts with five compact rows and no mounted editors, then reveals a bounded page and focused search', async () => {
    const { container } = mount()
    await screen.findByText('Team 001')
    expect(screen.getAllByRole('article')).toHaveLength(5)
    expect(container.querySelectorAll('form')).toHaveLength(0)
    fireEvent.click(
      screen.getByRole('button', { name: 'Show more · 295 remaining' })
    )
    const search = screen.getByRole('textbox', { name: /Search credentials/ })
    await waitFor(() => expect(search).toHaveFocus())
    expect(screen.getAllByRole('article')).toHaveLength(20)
    fireEvent.click(
      screen.getByRole('button', { name: 'Show more · 280 remaining' })
    )
    expect(screen.getAllByRole('article')).toHaveLength(40)
  })

  it('searches the whole inventory and restores the short list on collapse', async () => {
    mount()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Show more · 295 remaining' })
    )
    const search = screen.getByRole('textbox', { name: /Search credentials/ })
    fireEvent.change(search, { target: { value: 'team 299' } })
    expect(screen.getByText('Team 299')).toBeVisible()
    expect(screen.getAllByRole('article')).toHaveLength(1)
    expect(screen.getByRole('status')).toHaveTextContent('1 of 300')
    fireEvent.change(search, { target: { value: 'no such credential' } })
    expect(screen.getByText('No results')).toBeVisible()
    expect(screen.queryAllByRole('article')).toHaveLength(0)
    fireEvent.click(
      screen.getByRole('button', { name: 'Show fewer credentials' })
    )
    expect(screen.getAllByRole('article')).toHaveLength(5)
    expect(
      screen.queryByRole('textbox', { name: /Search credentials/ })
    ).not.toBeInTheDocument()
  })

  it('keeps an edited draft mounted while filtering, collapsing and reopening the editor', async () => {
    const { container } = mount()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Edit Team 001' })
    )
    const name = screen.getByLabelText('Name')
    fireEvent.change(name, { target: { value: 'unsaved team' } })
    fireEvent.click(screen.getByRole('button', { name: 'Edit Team 001' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Show more · 295 remaining' })
    )
    fireEvent.change(
      screen.getByRole('textbox', { name: /Search credentials/ }),
      { target: { value: '299' } }
    )
    expect(container.querySelectorAll('form')).toHaveLength(1)
    expect(dirty).toHaveBeenCalledWith('credential-1', true)
    fireEvent.click(
      screen.getByRole('button', { name: 'Show fewer credentials' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Edit Team 001' }))
    expect(screen.getByLabelText('Name')).toHaveValue('unsaved team')
    expect(network.update).not.toHaveBeenCalled()
  })

  it('does not offer list controls for a small inventory', async () => {
    mount(3)
    await screen.findByText('Team 001')
    expect(screen.getAllByRole('article')).toHaveLength(3)
    expect(
      screen.queryByRole('button', { name: /Show more/ })
    ).not.toBeInTheDocument()
  })

  it('searches by exact ID and credential type across unloaded rows', async () => {
    mount()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Show more · 295 remaining' })
    )
    const search = screen.getByRole('textbox', { name: /Search credentials/ })
    fireEvent.change(search, { target: { value: '  OAUTH  ' } })
    expect(screen.getByText('Team 250')).toBeVisible()
    expect(screen.getAllByRole('article')).toHaveLength(1)
    fireEvent.change(search, { target: { value: '250' } })
    expect(screen.getByText('Team 250')).toBeVisible()
  })

  it('preserves a late-row draft when collapsing and restoring the inventory', async () => {
    const { container } = mount()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Show more · 295 remaining' })
    )
    fireEvent.change(
      screen.getByRole('textbox', { name: /Search credentials/ }),
      { target: { value: '299' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Edit Team 299' }))
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'late-row draft' },
    })
    fireEvent.click(
      screen.getByRole('button', { name: 'Show fewer credentials' })
    )
    expect(container.querySelectorAll('form')).toHaveLength(1)
    expect(
      screen.queryByRole('textbox', { name: 'Name' })
    ).not.toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Show more · 295 remaining' })
    )
    fireEvent.change(
      screen.getByRole('textbox', { name: /Search credentials/ }),
      { target: { value: '299' } }
    )
    expect(screen.getByLabelText('Name')).toHaveValue('late-row draft')
    expect(network.update).not.toHaveBeenCalled()
  })

  it('distinguishes loading and a failed read from an empty inventory, then retries', async () => {
    let rejectRead!: (error: Error) => void
    network.credentials.mockImplementationOnce(
      () =>
        new Promise((_resolve, reject) => {
          rejectRead = reject
        })
    )
    mount(0)
    expect(screen.getByText('Loading…')).toBeVisible()
    expect(screen.queryByText('No credentials')).not.toBeInTheDocument()
    await act(async () => {
      rejectRead(new Error('Synthetic read failure'))
    })
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Synthetic read failure'
    )
    expect(screen.queryByText('No credentials')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('No credentials')).toBeVisible()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(network.credentials).toHaveBeenCalledTimes(2)
  })

  it('supports the Chinese interface language code when filtering names', async () => {
    await i18n.changeLanguage('zhCN')
    try {
      mount()
      await screen.findByText('Team 001')
      fireEvent.click(
        screen.getByRole('button', { name: '展开更多 · 剩余 295 个' })
      )
      fireEvent.change(screen.getByRole('textbox', { name: /查找凭据/ }), {
        target: { value: 'TEAM 299' },
      })
      expect(screen.getByText('Team 299')).toBeVisible()
      expect(screen.getAllByRole('article')).toHaveLength(1)
    } finally {
      await i18n.changeLanguage('en')
    }
  })
})
