import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import type { ModelRow } from '../../types'
import { ModelDetailSheet } from '../model-detail-sheet'

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  writeText: vi.fn(),
}))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => mocks.navigate }))
vi.mock('@/lib/toast', () => ({
  toast: { success: mocks.success, error: mocks.error },
}))

const originalClipboard = Object.getOwnPropertyDescriptor(
  navigator,
  'clipboard'
)
beforeEach(async () => {
  vi.clearAllMocks()
  mocks.writeText.mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: mocks.writeText },
  })
  await i18n.changeLanguage('zhCN')
})
afterEach(async () => {
  cleanup()
  if (originalClipboard)
    Object.defineProperty(navigator, 'clipboard', originalClipboard)
  else Reflect.deleteProperty(navigator, 'clipboard')
  await i18n.changeLanguage('en')
})

const model: ModelRow = {
  name: 'vendor/模型 + preview:1',
  description: '模型说明',
  accountCount: 0,
  tokenCount: 0,
  avgLatency: null,
  successRate: null,
  tags: [],
  supportedEndpointTypes: [],
  pricingSources: [],
  accounts: [],
}

it('opens at the model identity instead of focusing the tester action at the bottom', async () => {
  render(
    <ModelDetailSheet
      open
      onOpenChange={vi.fn()}
      model={{
        name: 'example-long-model-name',
        description: 'Model details',
        accountCount: 0,
        tokenCount: 0,
        avgLatency: null,
        successRate: null,
        tags: [],
        supportedEndpointTypes: [],
        pricingSources: [],
        accounts: [],
      }}
    />
  )
  const heading = await screen.findByRole('heading', {
    name: 'example-long-model-name',
  })
  await waitFor(() => expect(heading).toHaveFocus())
  expect(screen.getByRole('button', { name: '打开测试器' })).not.toHaveFocus()
})

it('puts copy and tester actions in the header and copies the exact model name', async () => {
  render(<ModelDetailSheet open model={model} onOpenChange={vi.fn()} />)
  const copy = await screen.findByRole('button', { name: '复制模型名' })
  expect(copy.closest('[data-slot=sheet-header]')).not.toBeNull()
  expect(
    screen
      .getByRole('button', { name: '打开测试器' })
      .closest('[data-slot=sheet-header]')
  ).not.toBeNull()
  fireEvent.click(copy)
  await waitFor(() =>
    expect(mocks.writeText).toHaveBeenCalledExactlyOnceWith(model.name)
  )
  await waitFor(() =>
    expect(mocks.success).toHaveBeenCalledExactlyOnceWith('已复制')
  )
  expect(mocks.error).not.toHaveBeenCalled()
})

it.each(['denied', 'unavailable'])(
  'reports clipboard failure and keeps details open (%s)',
  async (failure) => {
    if (failure === 'denied')
      mocks.writeText.mockRejectedValue(new Error('Permission denied'))
    else
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: undefined,
      })
    render(<ModelDetailSheet open model={model} onOpenChange={vi.fn()} />)
    fireEvent.click(await screen.findByRole('button', { name: '复制模型名' }))
    await waitFor(() =>
      expect(mocks.error).toHaveBeenCalledExactlyOnceWith('复制失败')
    )
    expect(mocks.success).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  }
)

it('opens the tester with the selected model name intact', async () => {
  const onOpenChange = vi.fn()
  render(<ModelDetailSheet open model={model} onOpenChange={onOpenChange} />)
  fireEvent.click(await screen.findByRole('button', { name: '打开测试器' }))
  expect(onOpenChange).toHaveBeenCalledWith(false)
  expect(mocks.navigate).toHaveBeenCalledExactlyOnceWith({
    href: `/model-tester?${new URLSearchParams({ model: model.name })}`,
    replace: true,
  })
})

it('lets readers expand and collapse long descriptions', async () => {
  const description = '模型能力与使用限制。'.repeat(25)
  render(
    <ModelDetailSheet
      open
      model={{ ...model, description }}
      onOpenChange={vi.fn()}
    />
  )
  const more = await screen.findByRole('button', { name: '展开全文' })
  expect(more).toHaveAttribute('aria-expanded', 'false')
  const text = screen.getByText(description)
  expect(text).toHaveClass('line-clamp-2')
  fireEvent.click(more)
  expect(screen.getByRole('button', { name: '收起' })).toHaveAttribute(
    'aria-expanded',
    'true'
  )
  expect(text).not.toHaveClass('line-clamp-2')
})
