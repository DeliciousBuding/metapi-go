import { formatCurrency } from '@/lib/format'

export const dimensions = [
  'input',
  'output',
  'cacheRead',
  'cacheWrite',
] as const
export function price(value: number | null | undefined, digits = 4) {
  if (value == null) return '—'
  return value > 0 && value < 10 ** -digits
    ? `$${value.toPrecision(4)}`
    : formatCurrency(value, { fractionDigits: digits })
}
