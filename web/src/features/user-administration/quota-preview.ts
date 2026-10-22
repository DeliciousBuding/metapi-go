const scale = 1_000_000n
const decimalPattern = /^(?:0|[1-9]\d*)(?:\.\d{1,6})?$/

function units(value: string): bigint | null {
  if (!decimalPattern.test(value)) return null
  const [whole = '0', fraction = ''] = value.split('.')
  return BigInt(whole) * scale + BigInt(fraction.padEnd(6, '0'))
}

function display(value: bigint): string {
  const sign = value < 0n ? '-' : ''
  const absolute = value < 0n ? -value : value
  const fraction = String(absolute % scale)
    .padStart(6, '0')
    .replace(/0+$/, '')
  return `${sign}${absolute / scale}${fraction ? `.${fraction}` : ''}`
}

/** Explanation only: server validation and the stored ledger remain authoritative. */
export function availableQuotaPreview(
  limit: string,
  used: number,
  reserved: number
) {
  const cap = units(limit.trim())
  const consumed = units(used.toFixed(6))
  const held = units(reserved.toFixed(6))
  if (
    cap === null ||
    cap > 1_000_000_000n * scale ||
    consumed === null ||
    held === null
  )
    return null
  return display(cap - consumed - held)
}

/** Fixed synthetic examples use integer micro-units, not floating-point billing. */
export function multiplierPreview(multiplier: string) {
  const value = units(multiplier.trim())
  if (value === null || value <= 0n || value > 1_000_000_000n * scale)
    return null
  const charge = (tokens: bigint) =>
    display((tokens * value + scale - 1n) / scale)
  return { fourteen: charge(14n), one: charge(1n) }
}
