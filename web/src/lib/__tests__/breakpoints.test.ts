// Guards the shared breakpoint constants (lib/breakpoints) and — critically —
// that the two responsive call sites actually consume them instead of
// re-hardcoding their own media-query strings. The whole point of the module
// is that the table→card-list switch and the sidebar→drawer switch cannot
// drift apart again, so the test fails if either consumer reintroduces a
// literal `(max-width: …px)` query.
import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import {
  SIDEBAR_MOBILE_MAX_WIDTH,
  SIDEBAR_MOBILE_MEDIA_QUERY,
  TABLE_MOBILE_MAX_WIDTH,
  TABLE_MOBILE_MEDIA_QUERY,
} from '@/lib/breakpoints'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')

function readSource(relativePath: string): string {
  return readFileSync(join(WEB_ROOT, relativePath), 'utf8')
}

describe('breakpoint constants', () => {
  it('keeps the documented 640px table / 768px sidebar thresholds', () => {
    expect(TABLE_MOBILE_MAX_WIDTH).toBe(640)
    // Sidebar flips at <768, encoded as a max-width of 767 so that a 768px
    // viewport is still desktop (mirrors the original innerWidth < 768 check).
    expect(SIDEBAR_MOBILE_MAX_WIDTH).toBe(767)
  })

  it('derives the media queries from the constants', () => {
    expect(TABLE_MOBILE_MEDIA_QUERY).toBe(
      `(max-width: ${TABLE_MOBILE_MAX_WIDTH}px)`
    )
    expect(SIDEBAR_MOBILE_MEDIA_QUERY).toBe(
      `(max-width: ${SIDEBAR_MOBILE_MAX_WIDTH}px)`
    )
  })

  it('documents the intentional 641-767px band (drawer nav + desktop table)', () => {
    const source = readSource('src/lib/breakpoints.ts')
    // The gap between the two thresholds is deliberate: mobile drawer
    // navigation combined with the desktop table (horizontal scroll). Guard
    // the rationale so a future "simplification" does not silently merge the
    // thresholds without a product decision.
    expect(source).toMatch(/641/)
    expect(source).toMatch(/767/)
    expect(source.toLowerCase()).toMatch(/drawer/)
  })
})

describe('consumers reference the shared constants', () => {
  it('DataTablePage uses TABLE_MOBILE_MEDIA_QUERY, not a hardcoded 640px query', () => {
    const source = readSource(
      'src/components/data-table/layout/data-table-page.tsx'
    )
    expect(source).toMatch(/from\s+'@\/lib\/breakpoints'/)
    expect(source).toContain('TABLE_MOBILE_MEDIA_QUERY')
    expect(source).not.toMatch(/\(max-width:\s*640px\)/)
  })

  it('useIsMobile uses SIDEBAR_MOBILE_MEDIA_QUERY, not a hardcoded 768/767 query', () => {
    const source = readSource('src/hooks/use-mobile.tsx')
    expect(source).toMatch(/from\s+'@\/lib\/breakpoints'/)
    expect(source).toContain('SIDEBAR_MOBILE_MEDIA_QUERY')
    expect(source).not.toMatch(/\(max-width:\s*768px\)/)
    expect(source).not.toMatch(/\(max-width:\s*767px\)/)
  })
})
