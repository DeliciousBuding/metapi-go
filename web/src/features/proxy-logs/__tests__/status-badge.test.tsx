// metapi-go/features/proxy-logs — StatusBadge label resolution tests.
//
// Covers the i18n fix (#869): known string statuses (success/failed/…)
// resolve to translated labels, numeric HTTP codes stay numeric, unknown
// strings fall back to the raw value, and missing status renders the
// neutral "unknown" label.

import { cleanup, render, screen } from '@testing-library/react'
import i18n from 'i18next'
import { afterEach, beforeAll, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { StatusBadge } from '../components/status-badge'

beforeAll(async () => {
  await i18n.changeLanguage('zhCN')
})

afterEach(() => {
  cleanup()
})

function renderBadge(props: {
  httpStatus?: number | null
  status?: string | null
}) {
  return render(<StatusBadge {...props} />)
}

describe('StatusBadge', () => {
  it('translates the "success" string status via i18n', () => {
    renderBadge({ status: 'success' })
    expect(screen.getByText('成功')).toBeTruthy()
  })

  it('translates the "failed" string status via i18n', () => {
    renderBadge({ status: 'failed' })
    expect(screen.getByText('失败')).toBeTruthy()
  })

  it('translates error-family statuses as failed', () => {
    renderBadge({ status: 'timeout' })
    expect(screen.getByText('失败')).toBeTruthy()
  })

  it('keeps numeric HTTP codes as the raw label', () => {
    renderBadge({ httpStatus: 200 })
    expect(screen.getByText('200')).toBeTruthy()
  })

  it('keeps numeric status strings as the raw label', () => {
    renderBadge({ status: '429' })
    expect(screen.getByText('429')).toBeTruthy()
  })

  it('falls back to the raw value for unknown statuses', () => {
    renderBadge({ status: 'banana' })
    expect(screen.getByText('banana')).toBeTruthy()
  })

  it('renders the unknown label when no status is provided', () => {
    renderBadge({})
    expect(screen.getByText('未知')).toBeTruthy()
  })

  it('prefers the HTTP status over a string status', () => {
    renderBadge({ httpStatus: 500, status: 'success' })
    expect(screen.getByText('500')).toBeTruthy()
  })
})
