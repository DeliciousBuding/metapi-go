import { beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { notifyCheckinResult } from '../notify-checkin-result'

const messages = vi.hoisted(() => ({
  success: vi.fn(),
  info: vi.fn(),
  error: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({ toast: messages }))

beforeEach(() => {
  vi.clearAllMocks()
})

describe('manual check-in result feedback', () => {
  it('shows the upstream reward after a successful check-in', () => {
    notifyCheckinResult(
      {
        success: true,
        status: 'success',
        skipped: false,
        message: '',
        reward: '+5',
      },
      i18n.t
    )
    expect(messages.success).toHaveBeenCalledWith(
      i18n.t('checkin.toast.success'),
      { description: i18n.t('checkin.toast.successReward', { reward: '+5' }) }
    )
  })

  it('keeps skipped and failed outcomes distinct', () => {
    notifyCheckinResult(
      {
        success: true,
        status: 'skipped',
        skipped: true,
        message: 'already checked in',
        reward: null,
      },
      i18n.t
    )
    expect(messages.info).toHaveBeenCalledWith(
      i18n.t('checkin.toast.skipped'),
      { description: 'already checked in' }
    )
    notifyCheckinResult(
      {
        success: false,
        status: 'failed',
        skipped: false,
        message: 'not supported',
        reward: null,
      },
      i18n.t
    )
    expect(messages.error).toHaveBeenCalledWith(
      i18n.t('checkin.toast.failed'),
      { description: 'not supported' }
    )
  })
})
