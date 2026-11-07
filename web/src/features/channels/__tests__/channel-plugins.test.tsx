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
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { ChannelPlugin, PluginBinding } from '@/lib/api/channel-plugins'

import { ChannelPluginBindings } from '../components/channel-plugin-bindings'
import { ChannelPluginManager } from '../components/channel-plugin-manager'

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  detail: vi.fn(),
  save: vi.fn(),
  toggle: vi.fn(),
  remove: vi.fn(),
}))
vi.mock('@/lib/api/channel-plugins', async (original) => ({
  ...(await original<typeof import('@/lib/api/channel-plugins')>()),
  channelPluginsApi: {
    list: mocks.list,
    detail: mocks.detail,
    save: mocks.save,
    setEnabled: mocks.toggle,
    remove: mocks.remove,
  },
}))
vi.mock('@/lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

function plugin(key: string): ChannelPlugin {
  return {
    key,
    revision: 3,
    enabled: true,
    manifest: {
      apiVersion: 1,
      key,
      name: `Vendor ${key}`,
      version: '1.0.0',
      protocols: ['chat', 'responses'],
      hooks: ['request'],
      options: [
        { key: 'mode', label: 'Vendor mode', type: 'string', default: 'safe' },
      ],
    },
  }
}
function mount(node: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
  return client
}
beforeEach(async () => {
  vi.resetAllMocks()
  await i18n.changeLanguage('en')
  mocks.list.mockResolvedValue({ items: [plugin('alpha'), plugin('beta')] })
  mocks.save.mockResolvedValue({
    key: 'vendor-options',
    revision: 1,
    enabled: false,
  })
})
afterEach(cleanup)

describe('channel plugin management and bindings', () => {
  it('explains explicit transport bindings in both supported languages', async () => {
    const english = i18n.t('channels.plugins.bindingDescription')
    await i18n.changeLanguage('zhCN')
    const chinese = i18n.t('channels.plugins.bindingDescription')
    expect(english).toContain('explicitly selected protocol endpoints')
    expect(english).toContain('WebSocket have separate bindings')
    expect(chinese).toContain('WebSocket 分别绑定')
  })

  it('offers native transport slots only when declared by both plugin and channel', async () => {
    const item = plugin('alpha')
    if (!item.manifest) throw new Error('missing fixture manifest')
    item.manifest.protocols = [
      'responses',
      'responsesCompact',
      'responsesWebsocket',
    ]
    mocks.list.mockResolvedValue({ items: [item] })
    const changed = vi.fn()
    mount(
      <ChannelPluginBindings
        value={[]}
        onChange={changed}
        endpoints={{
          responses: {
            url: 'https://example.invalid/responses',
            auth: 'bearer',
          },
          responsesWebsocket: {
            url: 'wss://example.invalid/responses',
            auth: 'bearer',
          },
        }}
      />
    )
    fireEvent.click(
      await screen.findByRole('checkbox', { name: 'Vendor alpha' })
    )
    expect(changed).toHaveBeenLastCalledWith([
      { key: 'alpha', protocols: ['responses', 'responsesWebsocket'] },
    ])
  })

  it('names the open option list from its unique visible label', async () => {
    const item = plugin('alpha')
    if (!item.manifest) throw new Error('missing fixture manifest')
    item.manifest.options = [
      {
        key: 'mode',
        label: 'Vendor mode',
        type: 'select',
        choices: ['safe', 'fast'],
        default: 'safe',
      },
    ]
    mocks.list.mockResolvedValue({ items: [item] })
    mount(
      <ChannelPluginBindings
        value={[{ key: 'alpha', protocols: ['chat'] }]}
        onChange={vi.fn()}
        endpoints={{
          chat: { url: 'https://example.invalid/chat', auth: 'bearer' },
        }}
      />
    )
    const trigger = await screen.findByRole('combobox', {
      name: 'Vendor mode',
    })
    fireEvent.click(trigger)
    const list = await screen.findByRole('listbox')
    expect(list).toHaveAccessibleName('Vendor mode')
    expect(within(list).getByRole('option', { name: 'safe' })).toBeVisible()
    expect(within(list).getByRole('option', { name: 'fast' })).toBeVisible()
  })

  it('derives protocols/default options from the manifest and gives repeated option labels unique IDs', async () => {
    const changed = vi.fn()
    function Bindings() {
      const [value, setValue] = useState<PluginBinding[]>([])
      return (
        <ChannelPluginBindings
          value={value}
          onChange={(next) => {
            changed(next)
            setValue(next)
          }}
          endpoints={{
            chat: { url: 'https://example.invalid/chat', auth: 'bearer' },
          }}
        />
      )
    }
    mount(<Bindings />)
    fireEvent.click(
      await screen.findByRole('checkbox', { name: 'Vendor alpha' })
    )
    fireEvent.click(screen.getByRole('checkbox', { name: 'Vendor beta' }))
    const modes = screen.getAllByLabelText('Vendor mode')
    expect(modes[0].id).not.toBe(modes[1].id)
    expect(modes[0]).toHaveValue('safe')
    fireEvent.change(modes[0], { target: { value: 'custom' } })
    expect(changed).toHaveBeenLastCalledWith([
      { key: 'alpha', protocols: ['chat'], options: { mode: 'custom' } },
      { key: 'beta', protocols: ['chat'] },
    ])
    fireEvent.click(
      screen.getByRole('button', { name: 'Run Vendor beta earlier' })
    )
    expect(changed).toHaveBeenLastCalledWith([
      { key: 'beta', protocols: ['chat'] },
      { key: 'alpha', protocols: ['chat'], options: { mode: 'custom' } },
    ])
    expect(
      screen.queryByRole('checkbox', { name: 'OpenAI Responses' })
    ).not.toBeInTheDocument()
  })

  it('preserves missing bindings until an explicit removal and displays metadata failures', async () => {
    mocks.list.mockResolvedValue({ items: [] })
    const changed = vi.fn()
    mount(
      <ChannelPluginBindings
        value={[{ key: 'missing', protocols: ['chat'] }]}
        onChange={changed}
        endpoints={{}}
      />
    )
    const missing = await screen.findByRole('checkbox', {
      name: 'missing · Unavailable',
    })
    expect(changed).not.toHaveBeenCalled()
    await waitFor(() => expect(missing).not.toBeDisabled())
    fireEvent.click(missing)
    expect(changed).toHaveBeenCalledWith([])
  })

  it('does not cache source code and uses the current detail revision for edits', async () => {
    mocks.detail.mockResolvedValue({
      key: 'alpha',
      revision: 4,
      enabled: true,
      source: 'export const original = "synthetic-source";',
    })
    const client = mount(<ChannelPluginManager />)
    const row = (await screen.findByText('Vendor alpha')).closest('section')
    if (!row) throw new Error('missing plugin row')
    fireEvent.click(
      within(row).getByRole('button', { name: 'Edit plugin source' })
    )
    const source = await screen.findByLabelText('JavaScript module')
    await waitFor(() =>
      expect(source).toHaveValue('export const original = "synthetic-source";')
    )
    fireEvent.change(source, {
      target: { value: 'export const changed = "synthetic-source";' },
    })
    await client.invalidateQueries({ queryKey: ['channel-plugins'] })
    expect(source).toHaveValue('export const changed = "synthetic-source";')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(mocks.save).toHaveBeenCalledWith('alpha', {
        source: 'export const changed = "synthetic-source";',
        enabled: true,
        expectedRevision: 4,
      })
    )
    expect(
      JSON.stringify(client.getQueryData(['channel-plugins']))
    ).not.toContain('synthetic-source')
  })

  it('keeps a source draft after an asynchronous save failure', async () => {
    mocks.save.mockRejectedValue(new Error('synthetic save failed'))
    mount(<ChannelPluginManager />)
    fireEvent.click(screen.getByRole('button', { name: 'Install plugin' }))
    const source = await screen.findByLabelText('JavaScript module')
    fireEvent.change(source, {
      target: { value: 'export const synthetic = true;' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(mocks.save).toHaveBeenCalledOnce())
    expect(source).toHaveValue('export const synthetic = true;')
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(
      await screen.findByText('Discard unsaved changes?')
    ).toBeInTheDocument()
  })

  it('reports a failed list instead of showing a successful empty state', async () => {
    mocks.list.mockRejectedValue(new Error('synthetic unavailable'))
    mount(<ChannelPluginManager />)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'synthetic unavailable'
    )
    expect(screen.queryByText('No plugins installed')).not.toBeInTheDocument()
  })
})
