// metapi-go/features/import — pure helpers for parsing pasted URLs and a
// lightweight canonicalization that mirrors the backend CanonicalizeSiteURL
// (strip query/fragment, trim trailing slash) for duplicate comparison only.

export function parseUrlLines(text: string): string[] {
  const seen = new Set<string>()
  const urls: string[] = []
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim()
    if (line === '') continue
    if (seen.has(line)) continue
    seen.add(line)
    urls.push(line)
  }
  return urls
}

export function canonicalizeUrl(raw: string): string {
  const trimmed = raw.trim()
  if (trimmed === '') return trimmed
  try {
    const url = new URL(trimmed.includes('://') ? trimmed : `https://${trimmed}`)
    url.search = ''
    url.hash = ''
    return url.toString().replace(/\/$/, '')
  } catch {
    return trimmed.replace(/\/$/, '')
  }
}
