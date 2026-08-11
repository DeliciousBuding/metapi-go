import { describe, expect, it } from 'vitest'

import { toCSV } from './csvExport.js'

describe('toCSV', () => {
  it('escapes commas, quotes, and newlines per RFC 4180', () => {
    const rows = [{ a: 'hello', b: 'a,b', c: 'he said "hi"\nnewline' }]
    const csv = toCSV(rows, [
      { key: 'a', header: 'A' },
      { key: 'b', header: 'B' },
      { key: 'c', header: 'C' },
    ])
    expect(csv).toBe('A,B,C\r\nhello,"a,b","he said ""hi""\nnewline"')
  })

  it('uses format transform when provided', () => {
    const rows = [{ n: 3 }]
    const csv = toCSV(rows, [{ key: 'n', header: 'N', format: (r) => r.n * 2 }])
    expect(csv).toBe('N\r\n6')
  })

  it('renders null/undefined as empty cells', () => {
    const rows = [{ a: null, b: undefined, c: 'x' }]
    const csv = toCSV(rows, [
      { key: 'a', header: 'A' },
      { key: 'b', header: 'B' },
      { key: 'c', header: 'C' },
    ])
    expect(csv).toBe('A,B,C\r\n,,x')
  })

  it('joins rows with CRLF', () => {
    const rows = [{ a: 1 }, { a: 2 }, { a: 3 }]
    const csv = toCSV(rows, [{ key: 'a', header: 'A' }])
    expect(csv).toBe('A\r\n1\r\n2\r\n3')
  })
})
