// Envelope semantics for POST /api/checkin/trigger.
//
// The backend answers 200 with `success: (failed == 0)` plus a real
// per-account `summary`. A partial completion (some accounts failed) is a
// real outcome that callers must surface — collapsing it into a thrown
// error hides the counts and leaves the UI with no honest feedback.
// Only an envelope failure that carries no summary at all is a hard error.

import { describe, expect, it } from 'vitest'

import { parseTriggerCheckinAllResult } from '../api'

describe('parseTriggerCheckinAllResult', () => {
  it('returns the envelope when every account succeeded', () => {
    const result = parseTriggerCheckinAllResult({
      success: true,
      status: 'completed',
      message: '签到执行完成',
      summary: { total: 2, success: 2, failed: 0, skipped: 0 },
    })

    expect(result.success).toBe(true)
    expect(result.summary).toMatchObject({ total: 2, failed: 0 })
  })

  it('returns the envelope on partial failure so callers can surface the summary', () => {
    const result = parseTriggerCheckinAllResult({
      success: false,
      status: 'completed',
      message: '签到执行完成',
      summary: { total: 3, success: 1, failed: 2, skipped: 0 },
    })

    expect(result.success).toBe(false)
    expect(result.summary).toMatchObject({ total: 3, success: 1, failed: 2 })
  })

  it('throws on an envelope failure that carries no summary', () => {
    expect(() =>
      parseTriggerCheckinAllResult({
        success: false,
        status: 'completed',
        message: 'checkin unavailable',
      })
    ).toThrow('checkin unavailable')
  })

  it('throws with the i18n fallback when the failure has no message either', () => {
    expect(() => parseTriggerCheckinAllResult({ success: false })).toThrow()
  })
})
