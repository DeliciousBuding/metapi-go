// metapi-go/data-table — columnLabel: the one rule for a column's human name.
//
// A column can be named three ways: a plain string `header` (`header: 'Name'`,
// usually already translated by the feature), a `meta.label` for when `header`
// is a component (a sort/filter control has no string to reuse), or neither —
// in which case there is no human name and the caller decides what to fall back
// to. Two places needed that rule and had already drifted on the last step (the
// card layout gave up with null, the column toggle showed the raw column id), so
// the rule lives here and each caller keeps its own fallback.

import type { ColumnDef } from '@tanstack/react-table'

/** The column's display name, or null when it does not have one. */
export function columnLabel<TData, TValue>(
  columnDef: ColumnDef<TData, TValue>
): string | null {
  const { header, meta } = columnDef

  if (typeof header === 'string') return header
  return meta?.label || null
}
