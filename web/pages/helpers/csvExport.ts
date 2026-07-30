// Minimal CSV export helpers for list pages. No dependency on a CSV library.
// RFC 4180-ish: quotes fields containing comma / quote / newline by wrapping
// in quotes and doubling embedded quotes. UTF-8 BOM so Excel renders CJK.

function csvEscape(value: unknown): string {
  if (value === null || value === undefined) return '';
  const s = typeof value === 'string' ? value : String(value);
  if (/[",\n\r]/.test(s)) {
    return `"${s.replace(/"/g, '""')}"`;
  }
  return s;
}

export type CsvColumn<T> = {
  key: keyof T & string;
  header: string;
  /** Optional value transform before escaping (e.g. format a number). */
  format?: (row: T) => unknown;
};

export function toCSV<T>(rows: T[], columns: CsvColumn<T>[]): string {
  const head = columns.map((c) => csvEscape(c.header)).join(',');
  const body = rows
    .map((row) =>
      columns
        .map((c) => csvEscape(c.format ? c.format(row) : row[c.key]))
        .join(','),
    )
    .join('\r\n');
  return `${head}\r\n${body}`;
}

export function downloadCSV(filename: string, csv: string): void {
  const blob = new Blob([`﻿${csv}`], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
