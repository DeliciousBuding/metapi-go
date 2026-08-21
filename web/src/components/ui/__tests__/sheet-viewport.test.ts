// Static guard for the Sheet small-screen contract, mirroring
// dialog-viewport.test.ts. The right/left panel must be full-width below the
// `sm` breakpoint (a 375px viewport gets an edge-to-edge panel, not the old
// 75%-wide strip), narrow to `w-3/4` capped by `sm:max-w-sm` at `sm+`
// (desktop unchanged), and provide a scrollable content area by default so
// tall content is scrollable instead of clipped.
import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../../..')
const SHEET_PATH = join(WEB_ROOT, 'src/components/ui/sheet.tsx')

function readSheetSource(): string {
  return readFileSync(SHEET_PATH, 'utf8')
}

/** Extract the class string of one side branch inside SheetContent. */
function extractSideClasses(source: string, side: 'right' | 'left'): string {
  const branch = source.match(new RegExp(`side === '${side}' &&\\s*'([^']+)'`))
  expect(
    branch,
    `expected a '${side}' side branch in SheetContent`
  ).not.toBeNull()
  return branch![1]
}

describe('sheet viewport safety', () => {
  it('right/left panels are full-width below sm and keep the sm sizing', () => {
    const source = readSheetSource()

    for (const side of ['right', 'left'] as const) {
      const classes = extractSideClasses(source, side)
      // <sm: edge-to-edge panel.
      expect(classes, `${side} side must be w-full below sm`).toMatch(
        /\bw-full\b/
      )
      // sm+: the previous desktop sizing is preserved unchanged.
      expect(classes, `${side} side must keep sm:w-3/4`).toMatch(/sm:w-3\/4/)
      expect(classes, `${side} side must keep sm:max-w-sm`).toMatch(
        /sm:max-w-sm/
      )
      // No residual all-viewport `w-3/4` (that was the 75%-wide mobile bug).
      expect(classes, `${side} side must not force w-3/4 below sm`).not.toMatch(
        /(^|\s)w-3\/4/
      )
    }
  })

  it('panel is a scroll container instead of clipping overflow', () => {
    const source = readSheetSource()

    // The shared panel classes must scroll tall content vertically.
    const popupMatch = source.match(
      /data-slot='sheet-content'[\s\S]*?className=\{cn\(\s*'([^']+)'/
    )
    expect(popupMatch).not.toBeNull()
    const panelClasses = popupMatch![1]
    expect(panelClasses).toMatch(/overflow-y-auto/)
    expect(panelClasses).not.toMatch(/overflow-hidden/)
  })

  it('panel stays a vertical flex column so sticky footers can use mt-auto', () => {
    const source = readSheetSource()

    const popupMatch = source.match(
      /data-slot='sheet-content'[\s\S]*?className=\{cn\(\s*'([^']+)'/
    )
    const panelClasses = popupMatch![1]
    expect(panelClasses).toMatch(/flex flex-col/)

    const footerMatch = source.match(
      /function SheetFooter[\s\S]*?className=\{cn\(([\s\S]*?)\)\}/
    )
    expect(footerMatch).not.toBeNull()
    // mt-auto pins the footer to the bottom of the flex column when the
    // content region is the scroll container (e.g. settings keys sheet).
    expect(footerMatch![1]).toMatch(/mt-auto/)
  })
})
