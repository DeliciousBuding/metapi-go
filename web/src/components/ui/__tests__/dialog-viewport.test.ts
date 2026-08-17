import { readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../../..')
const DIALOG_PATH = join(WEB_ROOT, 'src/components/ui/dialog.tsx')

describe('dialog viewport safety', () => {
  it('DialogContent caps height and scrolls when content overflows', () => {
    const source = readFileSync(DIALOG_PATH, 'utf8')

    // DialogContent must declare a max-height bound (using dvh/vh) so a
    // tall form cannot bleed past the viewport edges, and overflow-y-auto
    // so the capped content is scrollable instead of clipped.
    expect(source).toMatch(/max-h-\[calc\(100d?vh-\d+rem\)\]/)
    expect(source).toMatch(/overflow-y-auto/)
  })

  it('DialogFooter sticks to the bottom so action buttons stay visible', () => {
    const source = readFileSync(DIALOG_PATH, 'utf8')

    expect(source).toMatch(/sticky\s+bottom-0/)
  })

  it('DialogFooter uses an opaque background (no see-through on scroll)', () => {
    const source = readFileSync(DIALOG_PATH, 'utf8')
    const footerMatch = source.match(
      /function DialogFooter[\s\S]*?className=\{cn\(([\s\S]*?)\)\}/
    )
    expect(footerMatch).not.toBeNull()
    const footerClasses = footerMatch![1]
    // Semi-transparent bg (e.g. bg-muted/70) + backdrop-blur lets scrolling
    // content bleed through the sticky footer. Require a solid bg instead.
    expect(footerClasses).not.toMatch(/backdrop-blur/)
    expect(footerClasses).not.toMatch(/bg-\w+\/\d+/)
  })
})
