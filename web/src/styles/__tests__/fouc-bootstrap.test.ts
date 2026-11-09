import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { beforeEach, describe, expect, it } from 'vitest'

import { THEME_SCALE_COOKIE, THEME_SCALE_VALUES } from '@/lib/theme-scale'

const script = readFileSync(resolve('public/theme-init.js'), 'utf8')

beforeEach(() => {
  for (const name of [
    THEME_SCALE_COOKIE,
    'theme_preset',
    'theme_font',
    'theme_radius',
    'theme_content_layout',
  ]) {
    document.cookie = `${name}=; Max-Age=0; path=/`
  }
  document.body.removeAttribute('data-theme-scale')
})

describe('display-size bootstrap', () => {
  it.each([...THEME_SCALE_VALUES])(
    'applies stored %s size before mount',
    (scale) => {
      document.cookie = `${THEME_SCALE_COOKIE}=${scale}; path=/`
      window.eval(script)
      expect(document.body.getAttribute('data-theme-scale')).toBe(
        scale === 'default' ? null : scale
      )
    }
  )

  it('ignores an invalid size and retired decorative preferences', () => {
    document.cookie = `${THEME_SCALE_COOKIE}=unknown; path=/`
    document.cookie = 'theme_preset=cobalt; path=/'
    document.cookie = 'theme_font=serif; path=/'
    document.cookie = 'theme_radius=xl; path=/'
    document.cookie = 'theme_content_layout=centered; path=/'
    window.eval(script)
    for (const attribute of [
      'scale',
      'preset',
      'font',
      'radius',
      'content-layout',
    ]) {
      expect(document.body).not.toHaveAttribute(`data-theme-${attribute}`)
    }
  })
})
