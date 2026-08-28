// metapi-go/features/proxy-logs — column definition metadata tests.
//
// Locks two Wave-7 regression contracts:
//  1. Every hideable column carries a translated `meta.label`, so the
//     DataTableViewOptions "View" (column toggle) panel renders the same
//     localized header names as the column headers — never the internal
//     `column.id` fallback (`CreatedAt` / `Account` / `Site` / …).
//  2. The status cell renders the failure `errorMessage` (one line) and the
//     real `httpStatus` (502 instead of a hard-coded null) so a failed row
//     explains itself on the list first screen without opening the sheet.

import type { ColumnDef } from '@tanstack/react-table'
import { cleanup, render, screen } from '@testing-library/react'
import i18n from 'i18next'
import { useEffect } from 'react'
import { afterEach, beforeAll, describe, expect, it } from 'vitest'

import '@/i18n/config'

import {
  useProxyLogsColumns,
  type ProxyLogsColumnActions,
} from '../components/proxy-logs-columns'
import type { ProxyLog } from '../types'

const NOOP_ACTIONS: ProxyLogsColumnActions = { onView: () => {} }
const HIDABLE_COLUMN_IDS = [
  'createdAt',
  'account',
  'site',
  'model',
  'status',
  'latencyMs',
  'token',
  'retryCount',
]

beforeAll(async () => {
  await i18n.changeLanguage('zhCN')
})

afterEach(() => {
  cleanup()
})

/** Real component wrapper so `useProxyLogsColumns` runs as a proper hook. */
function ColumnsProbe({
  onReady,
}: {
  onReady: (columns: ColumnDef<ProxyLog>[]) => void
}) {
  const columns = useProxyLogsColumns(NOOP_ACTIONS)
  useEffect(() => {
    onReady(columns)
  }, [columns, onReady])
  return null
}

function loadColumns() {
  return new Promise<ColumnDef<ProxyLog>[]>((resolve) => {
    const handle = (cols: ColumnDef<ProxyLog>[]) => {
      resolve(cols)
    }
    render(<ColumnsProbe onReady={handle} />)
  })
}

describe('useProxyLogsColumns meta.labels', () => {
  it('every hideable column carries a non-empty translated label', async () => {
    const cols = await loadColumns()
    for (const column of cols) {
      if (column.enableHiding === false) continue
      expect(
        column.meta?.label,
        `column ${column.id} must set meta.label`
      ).toBeTruthy()
    }
  })

  it('labels are the same Chinese texts as the column headers', async () => {
    const cols = await loadColumns()
    const expected: Record<string, string> = {
      createdAt: '时间',
      account: '账号',
      site: '站点',
      model: '模型',
      status: '状态',
      latencyMs: '延迟',
      token: '令牌',
      retryCount: '重试',
    }
    for (const id of HIDABLE_COLUMN_IDS) {
      const column = cols.find((c) => c.id === id)
      expect(column?.meta?.label).toBe(expected[id])
    }
  })

  it('the actions column cannot be hidden (so no toggle entry is needed)', async () => {
    const cols = await loadColumns()
    const actions = cols.find((c) => c.id === 'actions')
    expect(actions?.enableHiding).toBe(false)
  })
})

describe('proxy-logs status cell failure visibility', () => {
  const failedLog: ProxyLog = {
    id: 7,
    createdAt: '2026-08-23T13:01:15+08:00',
    modelRequested: 'gpt-5.5',
    modelActual: 'gpt-5.5',
    status: 'failed',
    httpStatus: 502,
    latencyMs: 114,
    totalTokens: 0,
    retryCount: 1,
    username: 'svc-oneapi-02',
    siteName: 'OneAPI 聚合',
    errorMessage:
      'Post "https://oneapi.example.com/v1/responses": dial tcp: lookup no such host',
  }

  async function renderStatusCell(log: ProxyLog = failedLog) {
    const cols = await loadColumns()
    const statusCell = cols.find((c) => c.id === 'status')?.cell
    expect(typeof statusCell).toBe('function')
    // The cell only reads `row.original` — a shape-compatible stub suffices.
    const row = { original: log } as never
    render(
      <div>
        {typeof statusCell === 'function' && statusCell({ row } as never)}
      </div>
    )
  }

  it('shows the numeric http status on the badge (502, not hard-coded null)', async () => {
    await renderStatusCell()
    expect(screen.getByText('502')).toBeTruthy()
  })

  it('shows the failure errorMessage inline for failed rows', async () => {
    await renderStatusCell()
    expect(screen.getByText(/dial tcp/)).toBeTruthy()
  })

  it('renders no error line when errorMessage is empty', async () => {
    await renderStatusCell({ ...failedLog, errorMessage: null })
    expect(screen.queryByText(/dial tcp/)).toBeNull()
  })
})
