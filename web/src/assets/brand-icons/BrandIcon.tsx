/* eslint-disable react/only-export-components -- icon component co-located with brand helpers */
import { useEffect, useState, type CSSProperties } from 'react'

import {
  avatarLetters,
  getBrand,
  getBrandIconUrl,
  hashColor,
  normalizeBrandIconKey,
  type BrandInfo,
} from './brandRegistry.js'

export { getBrand } from './brandRegistry.js'

const BRAND_ICON_VERSION = '1.83.0'
const ICON_CDN = `https://registry.npmmirror.com/@lobehub/icons-static-png/${BRAND_ICON_VERSION}/files/dark`
const ICON_CDN_LIGHT = `https://registry.npmmirror.com/@lobehub/icons-static-png/${BRAND_ICON_VERSION}/files/light`

function useIconCdn() {
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
  return isDark ? ICON_CDN : ICON_CDN_LIGHT
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

function BrandGlyph({
  brand,
  model,
  icon,
  alt,
  size = 16,
  fallbackText,
  style,
}: BrandGlyphProps) {
  const cdn = useIconCdn()
  const resolvedBrand = brand || (model ? getBrand(model) : null)
  const resolvedIcon = normalizeBrandIconKey(
    icon || resolvedBrand?.icon || null
  )
  const [imgError, setImgError] = useState(false)

  useEffect(() => {
    setImgError(false)
  }, [resolvedIcon])

  if (resolvedIcon && !imgError) {
    const src = getBrandIconUrl(resolvedIcon, cdn)
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
        background: hashColor(fallback),
        color: 'var(--color-primary-foreground)',
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontSize: Math.max(9, Math.round(size * 0.5)),
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
  const brand = getBrand(model)

  if (brand) {
    return (
      <div
        style={{
          width: size,
          height: size,
          borderRadius: 10,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          flexShrink: 0,
          background: 'transparent',
        }}
      >
        <BrandGlyph brand={brand} size={size} fallbackText={brand.name} />
      </div>
    )
  }

  return (
    <div
      className='model-card-avatar'
      style={{
        width: size,
        height: size,
        background: hashColor(model),
        fontSize: size > 32 ? 16 : 10,
      }}
    >
      {avatarLetters(model)}
    </div>
  )
}

export function InlineBrandIcon({
  model,
  size = 16,
}: {
  model: string
  size?: number
}) {
  const brand = getBrand(model)
  if (!brand) return null
  return <BrandGlyph brand={brand} size={size} fallbackText={brand.name} />
}
