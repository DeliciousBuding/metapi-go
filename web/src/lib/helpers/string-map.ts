export type StringMapEntry = { key: string; value: string }

// Parse string tokens after JSON validation so duplicate keys survive instead
// of being silently collapsed by JSON.parse / Object.entries.
export function parseStringMap(text: string): StringMapEntry[] | null {
  if (!text.trim()) return []
  try {
    const parsed: unknown = JSON.parse(text)
    if (
      parsed === null ||
      typeof parsed !== 'object' ||
      Array.isArray(parsed)
    ) {
      return null
    }
    const entries: StringMapEntry[] = []
    const pair = /\s*("(?:[^"\\]|\\.)*")\s*:\s*("(?:[^"\\]|\\.)*")\s*/y
    const body = text.trim().slice(1, -1)
    let offset = 0
    while (body.slice(offset).trim()) {
      pair.lastIndex = offset
      const match = pair.exec(body)
      if (!match) return null
      entries.push({ key: JSON.parse(match[1]), value: JSON.parse(match[2]) })
      offset = pair.lastIndex
      if (offset < body.length) {
        if (body[offset] !== ',') return null
        offset++
      }
    }
    return entries
  } catch {
    return null
  }
}

export function stringMapEntryError(
  entries: StringMapEntry[],
  index: number
): 'emptyKey' | 'duplicateKey' | null {
  const key = entries[index].key
  if (!key.trim()) return 'emptyKey'
  return entries.some((entry, other) => other !== index && entry.key === key)
    ? 'duplicateKey'
    : null
}

export function isValidStringMap(text: string): boolean {
  const entries = parseStringMap(text)
  return (
    entries !== null &&
    entries.every((_, index) => !stringMapEntryError(entries, index))
  )
}

export function serializeStringMap(entries: StringMapEntry[]): string {
  if (!entries.length) return ''
  return `{\n${entries.map((entry) => `  ${JSON.stringify(entry.key)}: ${JSON.stringify(entry.value)}`).join(',\n')}\n}`
}

// Preserve legacy object values (including numeric / boolean headers) in JSON
// mode. Only the row editor requires string values.
export function isValidMapObject(text: string): boolean {
  if (!text.trim()) return true
  try {
    const parsed: unknown = JSON.parse(text)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return false
    }
    // Scan top-level key tokens without collapsing duplicates, including
    // objects whose values cannot be rendered as string rows.
    const keys: string[] = []
    let depth = 0
    for (const token of text.matchAll(/"(?:[^"\\]|\\.)*"|[{}[\]]/g)) {
      if (token[0] === '{' || token[0] === '[') depth++
      else if (token[0] === '}' || token[0] === ']') depth--
      else if (
        depth === 1 &&
        /^\s*:/.test(text.slice(token.index + token[0].length))
      ) {
        keys.push(JSON.parse(token[0]))
      }
    }
    return (
      keys.every((key) => key.trim() !== '') &&
      new Set(keys).size === keys.length
    )
  } catch {
    return false
  }
}
