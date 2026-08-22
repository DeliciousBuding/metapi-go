// metapi-go/lib/helpers — CSV formula-injection neutralizer.
//
// Spreadsheet apps (Excel / LibreOffice / Google Sheets) interpret cell
// content starting with `=`, `+`, `@`, TAB or CR as formulas or DDE
// commands, and `-` as a formula unless it parses as a negative number.
// Quoting does NOT prevent evaluation, so exports that may contain
// user-controlled text (proxy-log model names are downstream-caller-
// controlled) must prefix such cells with a single quote, which
// spreadsheet apps treat as the "text" marker. Follows the OWASP CSV
// injection mitigation guidance.

/**
 * Cells starting with `=`, `+`, `@`, TAB, CR or `-` are spreadsheet
 * formula/DDE risks. A leading `-` is the one exception-prone case: a
 * string that parses ENTIRELY as a number (`-5.5`, `-123`) is a plain
 * negative value and must stay spreadsheet-numeric (cost/token columns),
 * while anything else (`-cmd|...`, `-1-1`, bare `-`) still evaluates as a
 * formula in Excel and gets the prefix.
 */
const CSV_FORMULA_STARTER = /^[=+@\t\r-]/

export function neutralizeCsvFormulaCell(value: string): string {
  if (!value) return value
  if (value.startsWith('-') && !Number.isNaN(Number(value))) return value
  return CSV_FORMULA_STARTER.test(value) ? `'${value}` : value
}
