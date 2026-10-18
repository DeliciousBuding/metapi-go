import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { ModelTesterPage } from '../components/model-tester-page'

const network = vi.hoisted(() => ({ testChatSync: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  useSearch: () => ({ model: 'test-model' }),
}))
vi.mock('@/lib/api', () => ({ api: { testChatSync: network.testChatSync } }))
vi.mock('@/features/models', () => ({
  useModels: () => ({
    data: [{ name: 'test-model' }],
    error: null,
    isLoading: false,
  }),
}))
vi.mock('@/features/channels', () => ({
  channelsKeys: { list: () => ['channels'] },
  useChannels: () => ({
    data: [
      {
        id: 1,
        name: 'Test channel',
        enabled: true,
        site: { id: 1, name: 'Test site' },
      },
    ],
    error: null,
    isLoading: false,
  }),
}))
vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

beforeEach(async () => {
  await i18n.changeLanguage('en')
  network.testChatSync.mockReset()
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
  window.matchMedia = vi.fn().mockReturnValue({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })
  Element.prototype.scrollIntoView = vi.fn()
})
afterEach(cleanup)

function success(content: string) {
  return new Response(
    JSON.stringify({
      success: true,
      statusCode: 200,
      latencyMs: 123,
      error: null,
      truncatedBody: JSON.stringify({
        choices: [
          {
            message: { content, reasoning_content: 'Reasoning for this turn' },
            finish_reason: 'stop',
          },
        ],
        usage: { prompt_tokens: 10, completion_tokens: 20, total_tokens: 30 },
      }),
    })
  )
}

async function openChat(selectChannel = true) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ModelTesterPage />
    </QueryClientProvider>
  )
  if (!selectChannel) return screen.getByRole('textbox', { name: 'Prompt' })
  fireEvent.click(screen.getByRole('button', { name: 'Chat settings' }))
  fireEvent.click(await screen.findByRole('combobox', { name: 'Channel' }))
  const option = await screen.findByRole('option', { name: /Test channel/ })
  fireEvent.pointerDown(option)
  fireEvent.click(option)
  await waitFor(() =>
    expect(screen.getByRole('combobox', { name: 'Channel' })).toHaveTextContent(
      'Test channel'
    )
  )
  fireEvent.click(screen.getAllByRole('button', { name: /^Close$/ })[0])
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  return screen.getByRole('textbox', { name: 'Prompt' })
}

describe('chat playground', () => {
  it('uses a suggestion as an editable draft and opens settings for a missing channel', async () => {
    const prompt = await openChat(false)
    expect(
      screen.queryByRole('combobox', { name: 'Channel' })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Quick connectivity/ }))
    expect(prompt).not.toHaveValue('')
    expect(prompt).toHaveFocus()
    expect(network.testChatSync).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(
      await screen.findByRole('dialog', { name: 'Chat settings' })
    ).toBeInTheDocument()
    expect(
      await screen.findByText('Please select a channel.')
    ).toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getByRole('combobox', { name: 'Channel' })).toHaveFocus()
    )
    expect(network.testChatSync).not.toHaveBeenCalled()
  })

  it('keeps settings and the draft when the panel closes and reopens', async () => {
    const prompt = await openChat()
    fireEvent.change(prompt, { target: { value: 'Keep this draft' } })
    fireEvent.click(screen.getByRole('button', { name: 'Chat settings' }))
    fireEvent.change(
      await screen.findByRole('textbox', { name: 'System prompt' }),
      { target: { value: 'Be concise' } }
    )
    fireEvent.click(screen.getAllByRole('button', { name: /^Close$/ })[0])
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(prompt).toHaveValue('Keep this draft')
    fireEvent.click(screen.getByRole('button', { name: 'Chat settings' }))
    expect(
      await screen.findByRole('textbox', { name: 'System prompt' })
    ).toHaveValue('Be concise')
    expect(screen.getByRole('combobox', { name: 'Channel' })).toHaveTextContent(
      'Test channel'
    )
  })

  it('sends multiple turns and keeps each response and diagnostics on its message', async () => {
    network.testChatSync
      .mockImplementationOnce(() => Promise.resolve(success('**First reply**')))
      .mockImplementationOnce(() => Promise.resolve(success('Second reply')))
    const prompt = await openChat()
    fireEvent.change(prompt, { target: { value: 'First question' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(await screen.findByText('First reply')).toHaveClass('font-semibold')
    await waitFor(() => expect(prompt).toHaveValue(''))
    expect(prompt).toHaveFocus()
    fireEvent.change(prompt, { target: { value: 'Follow-up question' } })
    fireEvent.keyDown(prompt, { key: 'Enter', ctrlKey: true })
    await screen.findByText('Second reply')
    expect(network.testChatSync.mock.calls[1][0].messages).toEqual([
      { role: 'user', content: 'First question' },
      { role: 'assistant', content: '**First reply**' },
      { role: 'user', content: 'Follow-up question' },
    ])
    const history = screen.getByRole('log', { name: 'Conversation' })
    expect(
      within(history).getAllByText('Response details · raw data')
    ).toHaveLength(2)
    expect(
      within(history).getAllByText('Tokens: 30 (10 in / 20 out)')
    ).toHaveLength(2)
    fireEvent.click(screen.getByRole('button', { name: 'Clear session' }))
    fireEvent.click(screen.getByRole('button', { name: /^Clear$/ }))
    expect(screen.queryByText('First reply')).not.toBeInTheDocument()
    expect(screen.getByText('What would you like to try?')).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Model' })).toHaveTextContent(
      'test-model'
    )
  })

  it('keeps the prompt after failure and excludes the failed round when retrying', async () => {
    network.testChatSync
      .mockRejectedValueOnce(new Error('Upstream unavailable'))
      .mockImplementationOnce(() => Promise.resolve(success('Recovered')))
    const prompt = await openChat()
    fireEvent.change(prompt, { target: { value: 'Retry me' } })
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Upstream unavailable'
    )
    expect(prompt).toHaveValue('Retry me')
    fireEvent.click(screen.getByRole('button', { name: 'Send' }))
    await screen.findByText('Recovered')
    expect(network.testChatSync.mock.calls[1][0].messages).toEqual([
      { role: 'user', content: 'Retry me' },
    ])
  })

  it('stops the request, preserves the draft, and does not send while the IME is composing', async () => {
    network.testChatSync.mockImplementation(
      (_payload, signal: AbortSignal) =>
        new Promise((_resolve, reject) =>
          signal.addEventListener('abort', () =>
            reject(new DOMException('This operation was aborted', 'AbortError'))
          )
        )
    )
    const prompt = await openChat()
    fireEvent.change(prompt, { target: { value: '中文测试' } })
    fireEvent.keyDown(prompt, {
      key: 'Enter',
      ctrlKey: true,
      isComposing: true,
    })
    expect(network.testChatSync).not.toHaveBeenCalled()
    fireEvent.keyDown(prompt, { key: 'Enter', ctrlKey: true })
    const stop = await screen.findByRole('button', { name: 'Stop' })
    expect(screen.getByRole('button', { name: 'Clear session' })).toBeDisabled()
    expect(screen.getByText('Waiting for response…')).toBeInTheDocument()
    fireEvent.click(stop)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Request was stopped.'
    )
    await screen.findByRole('button', { name: 'Send' })
    expect(prompt).toHaveValue('中文测试')
    expect(prompt).toHaveFocus()
  })
})
