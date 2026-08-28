// CSV formula-injection neutralizer (OWASP-style): cells whose first
// characters spreadsheet apps interpret as formula/DDE starters must be
// prefixed with a single quote so exports stay inert when opened in
// Excel/LibreOffice.

import { describe, expect, it } from 'vitest'

import { neutralizeCsvFormulaCell } from '../csv-injection'

describe('neutralizeCsvFormulaCell', () => {
  it('prefixes formula starters (= + @) with a single quote', () => {
    expect(neutralizeCsvFormulaCell('=1+1')).toBe("'=1+1")
    expect(neutralizeCsvFormulaCell('+1+1')).toBe("'+1+1")
    expect(neutralizeCsvFormulaCell('@SUM(A1:A2)')).toBe("'@SUM(A1:A2)")
    expect(neutralizeCsvFormulaCell("=cmd|'/C calc'!A0")).toBe(
      "'=cmd|'/C calc'!A0"
    )
  })

  it('prefixes DDE-style dash payloads but keeps plain negative numbers', () => {
    expect(neutralizeCsvFormulaCell("-cmd|'/C calc'!A0")).toBe(
      "'-cmd|'/C calc'!A0"
    )
    expect(neutralizeCsvFormulaCell('-1-1')).toBe("'-1-1")
    expect(neutralizeCsvFormulaCell('-')).toBe("'-")
    // Negative amounts/tokens must stay spreadsheet-numeric.
    expect(neutralizeCsvFormulaCell('-5.5')).toBe('-5.5')
    expect(neutralizeCsvFormulaCell('-123')).toBe('-123')
    expect(neutralizeCsvFormulaCell('-.5')).toBe('-.5')
  })

  it('prefixes tab and carriage-return starters', () => {
    expect(neutralizeCsvFormulaCell('\t=1+1')).toBe("'\t=1+1")
    expect(neutralizeCsvFormulaCell('\r=1+1')).toBe("'\r=1+1")
  })

  it('leaves benign content untouched', () => {
    expect(neutralizeCsvFormulaCell('')).toBe('')
    expect(neutralizeCsvFormulaCell('gpt-5.5')).toBe('gpt-5.5')
    expect(neutralizeCsvFormulaCell('正常模型名🚀')).toBe('正常模型名🚀')
    expect(neutralizeCsvFormulaCell('2026-08-22 12:00:00')).toBe(
      '2026-08-22 12:00:00'
    )
    expect(neutralizeCsvFormulaCell('0.05')).toBe('0.05')
    // Inner formula characters are harmless — only the start triggers.
    expect(neutralizeCsvFormulaCell('a=1+1')).toBe('a=1+1')
  })
})
