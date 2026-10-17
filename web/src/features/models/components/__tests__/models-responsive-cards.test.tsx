import '@testing-library/jest-dom/vitest'
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { DataTablePage } from '@/components/data-table'
import i18n from '@/i18n/config'

import type { ModelRow } from '../../types'
import { useModelsColumns, type ModelsColumnActions } from '../models-columns'

const model: ModelRow = {
  name: 'claude-example',
  accountCount: 2,
  supplyCount: 3,
  tokenCount: 2,
  avgLatency: 250,
  successRate: 0.9,
  description: '用于检查移动端布局的模型说明',
  tags: ['vision'],
  supportedEndpointTypes: ['anthropic'],
  pricingSources: [],
  accounts: [],
}
const models = [model]

beforeEach(async () => {
  await i18n.changeLanguage('zhCN')
})
afterEach(async () => {
  cleanup()
  vi.unstubAllGlobals()
  await i18n.changeLanguage('en')
})

function mountCards(mobile: boolean, actions: ModelsColumnActions) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn((query: string) => ({
      matches: mobile,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }))
  )
  function Harness() {
    const columns = useModelsColumns(actions)
    const table = useReactTable({
      data: models,
      columns,
      getCoreRowModel: getCoreRowModel(),
    })
    return <DataTablePage table={table} toolbarProps={null} />
  }
  return render(<Harness />)
}

it('keeps the Chinese model card identity and both row actions reachable', async () => {
  const actions = { onView: vi.fn(), onTest: vi.fn() }
  mountCards(true, actions)
  expect(screen.getByText(model.name)).toBeInTheDocument()
  expect(screen.getByText('用于检查移动端布局的模型说明')).toBeInTheDocument()
  expect(screen.queryByText('Anthropic')).not.toBeInTheDocument()
  expect(screen.getByText('供给来源')).toBeInTheDocument()
  expect(screen.getByText('账号数')).toBeInTheDocument()
  const menuButton = screen.getByRole('button', {
    name: i18n.t('models.columns.rowActions'),
  })
  fireEvent.click(menuButton)
  fireEvent.click(
    await screen.findByRole('menuitem', {
      name: i18n.t('models.actions.viewDetails'),
    })
  )
  expect(actions.onView).toHaveBeenCalledWith(model)
  fireEvent.click(menuButton)
  fireEvent.click(
    await screen.findByRole('menuitem', {
      name: i18n.t('models.actions.testModel'),
    })
  )
  expect(actions.onTest).toHaveBeenCalledWith(model)
})

it('retains the brand and protocol columns in the Chinese desktop table', () => {
  mountCards(false, { onView: vi.fn(), onTest: vi.fn() })
  expect(screen.getByRole('table')).toBeInTheDocument()
  expect(screen.getByText('Anthropic')).toBeInTheDocument()
  expect(screen.getByText('anthropic')).toBeInTheDocument()
  expect(screen.getByRole('columnheader', { name: /品牌/ })).toBeInTheDocument()
})

it.each([true, false])(
  'opens model details directly from the name (mobile=%s)',
  (mobile) => {
    const actions = { onView: vi.fn(), onTest: vi.fn() }
    mountCards(mobile, actions)
    fireEvent.click(
      screen.getByRole('button', { name: `查看详情: ${model.name}` })
    )
    expect(actions.onView).toHaveBeenCalledExactlyOnceWith(model)
    expect(actions.onTest).not.toHaveBeenCalled()
  }
)
