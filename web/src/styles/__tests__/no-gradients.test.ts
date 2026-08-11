import { readdirSync, readFileSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const SCAN_ROOTS = [join(WEB_ROOT, 'src'), join(WEB_ROOT, 'public')]
const ROOT_TEXT_FILES = [join(WEB_ROOT, 'index.html')]
const TEXT_EXTENSIONS = new Set([
  '.css',
  '.html',
  '.js',
  '.json',
  '.jsx',
  '.mjs',
  '.svg',
  '.ts',
  '.tsx',
])
const THIS_FILE = fileURLToPath(import.meta.url)

const FORBIDDEN_PATTERNS = [
  {
    label: 'CSS color ramp',
    pattern: /(?:linear|radial|conic)-gradient\s*\(/i,
  },
  {
    label: 'SVG color ramp definition',
    pattern: /<\s*(?:linear|radial)Gradient\b/i,
  },
  { label: 'SVG color ramp reference', pattern: /url\(\s*#.*gradient/i },
  {
    label: 'Tailwind color ramp utility',
    pattern: /\bbg-(?:linear|radial)(?:-|\b)/i,
  },
]

function collectTextFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) return collectTextFiles(path)
    if (path === THIS_FILE || !TEXT_EXTENSIONS.has(extname(path))) return []
    return [path]
  })
}

describe('flat visual surfaces', () => {
  it('keeps application source and public assets free of color ramps', () => {
    const files = [...SCAN_ROOTS.flatMap(collectTextFiles), ...ROOT_TEXT_FILES]
    const violations = files.flatMap((path) => {
      const content = readFileSync(path, 'utf8')
      return FORBIDDEN_PATTERNS.filter(({ pattern }) =>
        pattern.test(content)
      ).map(({ label }) => `${relative(WEB_ROOT, path)}: ${label}`)
    })

    expect(violations).toEqual([])
  })

  it('keeps the sign-in background flat', () => {
    const signInPage = readFileSync(
      join(WEB_ROOT, 'src/features/auth/components/sign-in-page.tsx'),
      'utf8'
    )

    expect(signInPage).not.toContain('blur-3xl')
    expect(signInPage).not.toContain('bg-primary/10')
  })
})
