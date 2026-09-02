// Behavior tests for the shared copyText() guard: true on a successful
// write, false when the clipboard API is missing, false when the write is
// rejected (callers map false onto their failure toast).

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { copyText } from '../clipboard'

const writeTextMock = vi.fn()

beforeEach(() => {
  writeTextMock.mockReset()
  writeTextMock.mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', {
    writable: true,
    configurable: true,
    value: { writeText: writeTextMock },
  })
})

afterEach(() => {
  Object.defineProperty(navigator, 'clipboard', {
    writable: true,
    configurable: true,
    value: undefined,
  })
})

describe('copyText', () => {
  it('writes the text and resolves true', async () => {
    await expect(copyText('hello')).resolves.toBe(true)
    expect(writeTextMock).toHaveBeenCalledWith('hello')
  })

  it('resolves false when the write is rejected', async () => {
    writeTextMock.mockRejectedValue(new Error('denied'))
    await expect(copyText('hello')).resolves.toBe(false)
  })

  it('resolves false without touching writeText when clipboard is missing', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      writable: true,
      configurable: true,
      value: undefined,
    })
    await expect(copyText('hello')).resolves.toBe(false)
    expect(writeTextMock).not.toHaveBeenCalled()
  })
})
