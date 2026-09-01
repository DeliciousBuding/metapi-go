// metapi-go/lib — shared URL safety helpers used by list columns and
// forms across features (sites external links, accounts site cells, checkin
// sheets). Extracted from features/sites/lib/endpoints.ts (#1108 accounts
// quick jump) so both features consume one implementation instead of
// duplicating the validation ladder.

/** Hostnames the backend rejects as a first-hop SSRF risk (cloud metadata). */
const FORBIDDEN_METADATA_HOSTS = new Set([
  'metadata.google.internal',
  'metadata',
  'instance-data',
])

/** Matches a full IPv4 address (also usable for ::ffff:1.2.3.4 tails). */
const IPV4_RE = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/

/**
 * Cloud metadata / link-local targets the backend rejects as a first-hop
 * SSRF risk. Mirrors Go `IsForbiddenSiteTargetURL` — RFC1918 private and
 * localhost stay allowed for lab/docker operators.
 */
export function isForbiddenEndpointTargetHost(hostname: string): boolean {
  const bare = hostname
    .trim()
    .toLowerCase()
    .replaceAll(/^\[|\]$/g, '')
  if (FORBIDDEN_METADATA_HOSTS.has(bare)) return true
  const ipv4Tail = /::ffff:(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})$/.exec(bare)
  const v4 = ipv4Tail
    ? (ipv4Tail[1] as string).split('.')
    : IPV4_RE.exec(bare)?.slice(1)
  if (v4) {
    // Link-local IPv4 169.254.0.0/16 (includes AWS/GCP/Azure metadata).
    return Number(v4[0]) === 169 && Number(v4[1]) === 254
  }
  // IPv6 link-local unicast fe80::/10: the first hextet is fe8 */fe9 */fea */feb *.
  const firstSegment = bare.split(':')[0] ?? ''
  return /^fe[89ab][0-9a-f]$/.test(firstSegment)
}

/** Valid endpoint URL: http(s) and not a forbidden metadata/link-local target. */
export function isValidEndpointUrl(raw: string): boolean {
  const trimmed = raw.trim()
  if (trimmed === '') return false
  try {
    const parsed = new URL(trimmed)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      return false
    }
    return !isForbiddenEndpointTargetHost(parsed.hostname)
  } catch {
    return false
  }
}
