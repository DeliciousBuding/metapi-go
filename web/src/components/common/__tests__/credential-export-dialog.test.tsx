import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
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
import type { CredentialExportResponse } from '@/lib/api/credential-export'
import { copyText } from '@/lib/clipboard'

import { CredentialExportDialog } from '../credential-export-dialog'

vi.mock('@/lib/api', () => ({ api: { getDownstreamKeyExport: vi.fn() } }))
vi.mock('@/lib/clipboard', () => ({ copyText: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children }: { to: string; children: ReactNode }) => (
    <a href={to}>{children}</a>
  ),
}))

const exportData: CredentialExportResponse = {
  success: true,
  formatVersion: '1',
  keyId: 1,
  keyName: 'demo-key',
  baseUrl: 'http://gateway.example.com',
  profiles: [
    {
      id: 'generic',
      label: 'Generic JSON',
      contentType: 'application/json',
      content: { apiKey: 'sk-plain-demo' },
    },
  ],
}
const target = { id: 1, name: 'demo-key', keyMasked: 'sk-***' }
const getExport = vi.mocked(api.getDownstreamKeyExport)
const { mockFetch } = vi.hoisted(() => ({ mockFetch: vi.fn() }))
vi.stubGlobal('fetch', mockFetch)

beforeEach(() => {
  getExport.mockReset()
  getExport.mockResolvedValue(exportData)
  mockFetch.mockReset()
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
    expect(screen.getByRole('combobox', { name: 'Client' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'CC Switch' })).toBeNull()
    expect(screen.queryByText('sk-plain-demo')).toBeNull()
    fireEvent.click(screen.getByRole('tab', { name: 'Test request' }))
    expect(screen.getByText('Verify the connection')).toBeVisible()
    expect(getExport).toHaveBeenCalledWith(
      1,
      'all',
      'master-token',
      undefined,
      expect.any(AbortSignal)
    )
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
    expect(getExport).toHaveBeenLastCalledWith(
      1,
      'all',
      'master-token',
      undefined,
      expect.any(AbortSignal)
    )
  })
})

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function exportForModel(model?: string): CredentialExportResponse {
  return {
    ...exportData,
    model,
    profiles: [
      ...exportData.profiles,
      {
        id: 'openai',
        label: 'OpenAI-compatible env',
        contentType: 'text/plain',
        content: 'export OPENAI_API_KEY=synthetic-key',
        protocol: 'chat',
      },
      {
        id: 'cherry',
        label: 'Cherry Studio',
        contentType: 'application/json',
        content: { apiKey: 'sk-plain-demo' },
        protocol: 'chat',
      },
      {
        id: 'codex',
        label: 'OpenAI Codex CLI',
        contentType: 'text/plain',
        content: `model = ${JSON.stringify(model ?? '')}\n[model_providers.metapi]\nenv_key = "METAPI_API_KEY"\nwire_api = "responses"`,
        protocol: 'responses',
        setup: {
          contentType: 'text/plain',
          content: 'export METAPI_API_KEY=synthetic-key',
        },
      },
      {
        id: 'claude-code',
        label: 'Claude Code',
        contentType: 'application/json',
        content: JSON.stringify({
          model,
          env: {
            ANTHROPIC_BASE_URL: exportData.baseUrl,
            ANTHROPIC_AUTH_TOKEN: 'synthetic-key',
          },
        }),
        protocol: 'messages',
      },
    ],
  }
}

async function unlockWithModels() {
  getExport.mockImplementation(async (_id, _profile, _token, model) =>
    exportForModel(model)
  )
  const view = renderDialog()
  unlock()
  await screen.findByText('Verify the connection')
  mockFetch.mockResolvedValueOnce(
    jsonResponse(200, {
      data: [{ id: 'gpt-5.5' }, { id: 'claude-x' }],
    })
  )
  fireEvent.click(
    screen.getByRole('button', { name: 'Check available models' })
  )
  await screen.findByText('2 models listed for this key')
  return view
}

async function selectOption(ariaName: string, option: string) {
  fireEvent.click(screen.getByRole('combobox', { name: ariaName }))
  const item = await screen.findByRole('option', { name: option })
  fireEvent.pointerDown(item, { pointerType: 'mouse' })
  fireEvent.click(item)
}

describe('shared connection context', () => {
  it('keeps generic provider configurations separate from named client imports', async () => {
    await unlockWithModels()
    fireEvent.click(screen.getByRole('tab', { name: 'Client setup' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Update configuration' })
    )
    await screen.findByText('Set the API key')
    await selectOption('Client', 'OpenAI-compatible env')
    expect(screen.getByRole('button', { name: 'Copy' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'CC Switch' })).toBeNull()
    await selectOption('Client', 'Generic JSON')
    expect(screen.getByRole('button', { name: 'Copy' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'CC Switch' })).toBeNull()
    expect(mockFetch).toHaveBeenCalledTimes(1)
  })

  it('rejects a configuration response for a different public model', async () => {
    await unlockWithModels()
    getExport.mockResolvedValueOnce(exportForModel('other-model'))
    fireEvent.click(screen.getByRole('tab', { name: 'Client setup' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Update configuration' })
    )
    expect(
      await screen.findByText('Failed to load credential export.')
    ).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Copy' })).toBeNull()
  })

  it('uses one public model for the explicit export and native client check, with a separate Codex setup step', async () => {
    await unlockWithModels()
    expect(screen.getByRole('radio', { name: 'Responses' })).toBeChecked()
    expect(getExport).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('tab', { name: 'Client setup' }))
    expect(screen.queryByRole('button', { name: 'Copy' })).toBeNull()
    fireEvent.click(
      screen.getByRole('button', { name: 'Update configuration' })
    )
    await screen.findByText('Set the API key')
    expect(getExport).toHaveBeenLastCalledWith(
      1,
      'all',
      'master-token',
      'gpt-5.5',
      expect.any(AbortSignal)
    )
    const copyButtons = screen.getAllByRole('button', { name: 'Copy' })
    fireEvent.click(copyButtons[0])
    await waitFor(() =>
      expect(copyText).toHaveBeenCalledWith(
        expect.stringContaining('model = "gpt-5.5"')
      )
    )
    expect(vi.mocked(copyText).mock.calls[0][0]).not.toContain('synthetic-key')
    fireEvent.click(copyButtons[1])
    await waitFor(() =>
      expect(copyText).toHaveBeenLastCalledWith(
        'export METAPI_API_KEY=synthetic-key'
      )
    )
    fireEvent.click(screen.getByRole('tab', { name: 'Test request' }))
    mockFetch.mockResolvedValueOnce(
      jsonResponse(200, {
        object: 'response',
        status: 'completed',
        output: [
          {
            type: 'message',
            role: 'assistant',
            content: [{ type: 'output_text', text: 'pong' }],
          },
        ],
      })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Send test request' }))
    await screen.findByText(/Responses request succeeded in/)
    const [url, init] = mockFetch.mock.calls[1] as [string, RequestInit]
    expect(url).toBe('http://gateway.example.com/v1/responses')
    expect(JSON.parse(String(init.body)).model).toBe('gpt-5.5')
  })

  it('invalidates a copied configuration and prior check when the public model changes', async () => {
    await unlockWithModels()
    fireEvent.click(screen.getByRole('tab', { name: 'Client setup' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Update configuration' })
    )
    await screen.findByText('Set the API key')
    fireEvent.click(screen.getByRole('tab', { name: 'Test request' }))
    await selectOption('Model for the test request', 'claude-x')
    expect(mockFetch).toHaveBeenCalledTimes(1)
    expect(getExport).toHaveBeenCalledTimes(2)
    fireEvent.click(screen.getByRole('tab', { name: 'Client setup' }))
    expect(screen.getByText(/selected model changed/)).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Copy' })).toBeNull()
    expect(screen.getByRole('button', { name: 'CC Switch' })).toBeDisabled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Update configuration' })
    )
    await screen.findByText('Set the API key')
    expect(getExport).toHaveBeenLastCalledWith(
      1,
      'all',
      'master-token',
      'claude-x',
      expect.any(AbortSignal)
    )
  })

  it('cancels an export when its model changes and cannot display the late old configuration', async () => {
    await unlockWithModels()
    let finish!: (value: CredentialExportResponse) => void
    getExport.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve
      })
    )
    fireEvent.click(screen.getByRole('tab', { name: 'Client setup' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Update configuration' })
    )
    const signal = getExport.mock.calls[1][4]
    fireEvent.click(screen.getByRole('tab', { name: 'Test request' }))
    await selectOption('Model for the test request', 'claude-x')
    expect(signal?.aborted).toBe(true)
    await act(async () => finish(exportForModel('gpt-5.5')))
    fireEvent.click(screen.getByRole('tab', { name: 'Client setup' }))
    expect(screen.queryByRole('button', { name: 'Copy' })).toBeNull()
    expect(screen.getByText(/selected model changed/)).toBeVisible()
  })

  it('clears the selected model and disables configuration export after a model-list refresh fails', async () => {
    await unlockWithModels()
    mockFetch.mockResolvedValueOnce(
      jsonResponse(401, { error: { message: 'key disabled' } })
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )
    await screen.findByText(/HTTP 401: key disabled/)
    fireEvent.click(screen.getByRole('tab', { name: 'Client setup' }))
    expect(
      screen.getByRole('button', { name: 'Update configuration' })
    ).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Copy' })).toBeNull()
  })

  it('changes the check protocol with the selected client and keeps deep-link confirmation free of implicit clipboard writes', async () => {
    await unlockWithModels()
    await selectOption('Client', 'Claude Code')
    expect(
      screen.getByRole('radio', { name: 'Anthropic Messages' })
    ).toBeChecked()
    fireEvent.click(screen.getByRole('button', { name: 'Copy endpoint' }))
    await waitFor(() =>
      expect(copyText).toHaveBeenCalledWith(exportData.baseUrl)
    )
    vi.mocked(copyText).mockClear()
    expect(mockFetch).toHaveBeenCalledTimes(1)
    expect(getExport).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('tab', { name: 'Client setup' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Update configuration' })
    )
    await screen.findByRole('button', { name: 'Copy' })
    fireEvent.click(screen.getByRole('button', { name: 'CC Switch' }))
    expect(await screen.findByText('Open deep link?')).toBeVisible()
    expect(copyText).not.toHaveBeenCalled()
    await selectOption('Client', 'OpenAI Codex CLI')
    expect(screen.queryByText('Open deep link?')).toBeNull()
    expect(getExport).toHaveBeenCalledTimes(2)
  })

  it('clears an earlier check when changing clients that share Chat Completions', async () => {
    await unlockWithModels()
    await selectOption('Client', 'OpenAI-compatible env')
    mockFetch.mockResolvedValueOnce(
      jsonResponse(200, {
        choices: [{ message: { role: 'assistant', content: 'pong' } }],
      })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Send test request' }))
    await screen.findByText(/Chat Completions request succeeded in/)
    await selectOption('Client', 'Cherry Studio')
    expect(screen.queryByText(/request succeeded in/)).toBeNull()
    expect(mockFetch).toHaveBeenCalledTimes(2)
  })

  it('aborts a model-specific export on close and leaves no credential cache behind', async () => {
    const view = await unlockWithModels()
    let finish!: (value: CredentialExportResponse) => void
    getExport.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve
      })
    )
    fireEvent.click(screen.getByRole('tab', { name: 'Client setup' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Update configuration' })
    )
    const signal = getExport.mock.calls[1][4]
    view.rerender(null)
    expect(signal?.aborted).toBe(true)
    await act(async () => finish(exportForModel('gpt-5.5')))
    expect(view.client.getQueryCache().getAll()).toHaveLength(0)
    expect(view.client.getMutationCache().getAll()).toHaveLength(0)
    view.rerender(target)
    expect(screen.getByPlaceholderText('Paste the admin token')).toHaveValue('')
    expect(screen.queryByText('gpt-5.5')).toBeNull()
  })
})
