// Static guard for the site-wide retry vocabulary (#889 visual consistency):
// every "try again after an error" button renders RefreshCw, the shared
// refresh glyph. RotateCw/RotateCcw (reset semantics) and TriangleAlert
// (warning semantics) previously leaked onto retry buttons and read as
// different actions on every surface.
import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../../..')

function readSource(relativePath: string): string {
  return readFileSync(join(WEB_ROOT, relativePath), 'utf8')
}

describe('retry button icon vocabulary', () => {
  it('QueryErrorBanner retry button uses RefreshCw, not TriangleAlert', () => {
    const source = readSource('src/components/common/query-error-banner.tsx')

    expect(source).toMatch(
      /import\s*\{[^}]*RefreshCw[^}]*\}\s*from\s*'lucide-react'/
    )

    // The Retry button region (from the onRetry handler to its closing tag)
    // must show the refresh glyph; the warning triangle stays on the banner
    // strip itself, where it is semantically correct.
    const retryButtonRegion = source.match(
      /onClick=\{onRetry\}[\s\S]*?<\/Button>/
    )
    expect(retryButtonRegion).not.toBeNull()
    const retryButton = retryButtonRegion?.[0] ?? ''
    expect(retryButton).toMatch(/<RefreshCw/)
    expect(retryButton).not.toMatch(/<TriangleAlert/)
  })

  it('the route-level ErrorPage retry button uses RefreshCw', () => {
    const source = readSource('src/components/layout/error-page.tsx')

    expect(source).toMatch(/<RefreshCw/)
    expect(source).not.toMatch(/<RotateCw/)
    expect(source).not.toMatch(/<RotateCcw/)
  })

  it('the LayoutErrorBoundary retry button uses RefreshCw', () => {
    const source = readSource('src/components/layout/layout-error-boundary.tsx')

    expect(source).toMatch(/<RefreshCw/)
    expect(source).not.toMatch(/<RotateCw/)
    expect(source).not.toMatch(/<RotateCcw/)
  })

  it('the routes page load-error retry is delegated to QueryErrorBanner', () => {
    // W19-T1 P2-o unification: the page no longer hand-rolls its own retry
    // button; the glyph vocabulary is pinned on QueryErrorBanner itself (see
    // the first test), so the page only needs to delegate, never regress to
    // a reset-style icon.
    const source = readSource(
      'src/features/token-routes/components/routes-page.tsx'
    )

    expect(source).toMatch(/<QueryErrorBanner/)
    expect(source).not.toMatch(/<RotateCw/)
    expect(source).not.toMatch(/<RotateCcw/)
  })
})
