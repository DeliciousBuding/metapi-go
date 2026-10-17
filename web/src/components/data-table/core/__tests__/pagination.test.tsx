import '@testing-library/jest-dom/vitest'
import {
  getCoreRowModel,
  useReactTable,
  type PaginationState,
} from '@tanstack/react-table'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import i18n from '@/i18n/config'

import { DataTablePagination } from '../pagination'

const data = [{ name: 'Model' }]
const columns = [{ accessorKey: 'name' }]

function Harness({ rowCount = 2000, pageIndex = 24 } = {}) {
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex,
    pageSize: 20,
  })
  const table = useReactTable({
    data,
    columns,
    rowCount,
    manualPagination: true,
    state: { pagination },
    onPaginationChange: setPagination,
    getCoreRowModel: getCoreRowModel(),
  })
  return <DataTablePagination table={table} />
}

beforeEach(async () => {
  await i18n.changeLanguage('zhCN')
})
afterEach(async () => {
  cleanup()
  await i18n.changeLanguage('en')
})

describe('shared pagination', () => {
  it('keeps the summary, selected page and navigation in sync', () => {
    render(<Harness />)
    expect(screen.getByRole('navigation', { name: '分页' })).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('第 25 / 100 页')
    expect(
      screen.getByRole('button', { name: /前往第 25 页/ })
    ).toHaveAttribute('aria-current', 'page')

    fireEvent.click(screen.getByRole('button', { name: '下一页' }))
    expect(screen.getByRole('status')).toHaveTextContent('第 26 / 100 页')
    expect(
      screen.getByRole('button', { name: /前往第 26 页/ })
    ).toHaveAttribute('aria-current', 'page')
    fireEvent.click(screen.getByRole('button', { name: '上一页' }))
    expect(screen.getByRole('status')).toHaveTextContent('第 25 / 100 页')
    fireEvent.click(screen.getByRole('button', { name: /前往第 100 页/ }))
    expect(screen.getByRole('status')).toHaveTextContent('第 100 / 100 页')
    expect(screen.getByRole('button', { name: '下一页' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '最后一页' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: '第一页' }))
    expect(screen.getByRole('status')).toHaveTextContent('第 1 / 100 页')
    expect(screen.getByRole('button', { name: '上一页' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '第一页' })).toBeDisabled()
  })

  it('changes page size through the real selector', async () => {
    render(<Harness />)
    fireEvent.click(screen.getByRole('combobox', { name: '每页行数' }))
    const option = await screen.findByRole('option', { name: '50' })
    fireEvent.pointerDown(option)
    fireEvent.click(option)
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('第 10 / 40 页')
    )
    expect(
      screen.getByRole('combobox', { name: '每页行数' })
    ).toHaveTextContent('50')
  })

  it('updates labels when the language changes', async () => {
    await i18n.changeLanguage('en')
    render(<Harness />)
    expect(
      screen.getByRole('navigation', { name: 'Pagination' })
    ).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('Page 25 of 100')
  })

  it('hides an empty first page', () => {
    render(<Harness rowCount={0} pageIndex={0} />)
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
  })

  it('keeps a way back when a later page becomes empty', () => {
    render(<Harness rowCount={0} pageIndex={2} />)
    fireEvent.click(screen.getByRole('button', { name: '上一页' }))
    expect(screen.getByRole('navigation')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '上一页' }))
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
  })
})
