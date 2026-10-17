/* eslint-disable react/only-export-components -- icon component co-located with brand helpers */
import { useEffect, useState, type CSSProperties } from 'react'

import {
  avatarLetters,
  getBrand,
  getBrandIconUrl,
  normalizeBrandIconKey,
  type BrandInfo,
} from './brandRegistry.js'

export { getBrand } from './brandRegistry.js'

/**
 * Tracks the active theme so brand glyphs can pick the matching dark/light
 * variant of the vendored icon set (see scripts/fetch-brand-icons.mjs).
 */
function useIsDarkTheme() {
  const [isDark, setIsDark] = useState(() => {
    if (typeof document === 'undefined') return false
    return document.documentElement.getAttribute('data-theme') === 'dark'
  })
  useEffect(() => {
    if (
      typeof document === 'undefined' ||
      typeof MutationObserver === 'undefined'
    ) {
      return undefined
    }
    const observer = new MutationObserver(() => {
      setIsDark(document.documentElement.getAttribute('data-theme') === 'dark')
    })
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme'],
    })
    return () => observer.disconnect()
  }, [])
  return isDark
}

type BrandGlyphProps = {
  brand?: Pick<BrandInfo, 'name' | 'icon'> | null
  model?: string | null
  icon?: string | null
  alt?: string
  size?: number
  fallbackText?: string | null
  style?: CSSProperties
}

export function BrandGlyph({
  brand,
  model,
  icon,
  alt,
  size = 16,
  fallbackText,
  style,
}: BrandGlyphProps) {
  const isDark = useIsDarkTheme()
  const resolvedBrand = brand || (model ? getBrand(model) : null)
  const resolvedIcon = normalizeBrandIconKey(
    icon || resolvedBrand?.icon || null
  )
  const [imgError, setImgError] = useState(false)

  useEffect(() => {
    setImgError(false)
  }, [resolvedIcon])

  if (resolvedIcon && !imgError) {
    const src = getBrandIconUrl(resolvedIcon, isDark ? 'dark' : 'light')
    if (src) {
      return (
        <img
          src={src}
          alt={alt || resolvedBrand?.name || model || 'brand'}
          style={{
            width: size,
            height: size,
            objectFit: 'contain',
            flexShrink: 0,
            verticalAlign: 'middle',
            ...style,
          }}
          onError={() => setImgError(true)}
          loading='lazy'
        />
      )
    }
  }

  const fallback = (fallbackText ?? resolvedBrand?.name ?? model ?? '').trim()
  if (!fallback) return null

  return (
    <span
      aria-hidden='true'
      style={{
        width: size,
        height: size,
        borderRadius: Math.max(4, Math.round(size * 0.33)),
        background: 'var(--color-secondary)',
        color: 'var(--color-secondary-foreground)',
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontSize: Math.max(8, Math.floor(size * 0.4)),
        fontWeight: 700,
        lineHeight: 1,
        flexShrink: 0,
        overflow: 'hidden',
        ...style,
      }}
    >
      {avatarLetters(fallback)}
    </span>
  )
}

export function BrandIcon({
  model,
  size = 44,
}: {
  model: string
  size?: number
}) {
  return <BrandGlyph model={model} size={size} />
}
