// metapi-go/components/common — client pill: which downstream client made the
// proxied call (Codex CLI, Claude Code, openai-node, ...), as a compact
// capsule with the brand glyph when one is known. The detection itself
// happens upstream (proxy/profiles); this component is display-only and never
// renders an empty pill.

import { BrandGlyph, getBrand } from '@/assets/brand-icons/BrandIcon'
import { cn } from '@/lib/utils'

/**
 * Client display names are app names ("Claude Code"), not model ids, so the
 * model-keyword rules in getBrand miss some of them ("Codex CLI" does not
 * start with "codex-mini"). Map the known client families explicitly first,
 * then fall back to the model-brand rules (covers e.g. "deepseek" tooling).
 */
const CLIENT_ICON_RULES: ReadonlyArray<readonly [RegExp, string]> = [
  [/claude/i, 'claude-color'],
  [/codex|openai|chatgpt/i, 'openai'],
  [/gemini|google/i, 'gemini-color'],
  [/deepseek/i, 'deepseek-color'],
  [/qwen|tongyi/i, 'qwen-color'],
]

function clientIconKey(name: string): string | null {
  for (const [pattern, icon] of CLIENT_ICON_RULES) {
    if (pattern.test(name)) return icon
  }
  return getBrand(name)?.icon ?? null
}

export type ClientPillProps = {
  /** Detected app display name (e.g. "Codex CLI"); preferred over family. */
  appName?: string | null
  /** Client family from UA sniffing (e.g. "openai-node"); fallback label. */
  family?: string | null
  title?: string
  className?: string
}

export function ClientPill({
  appName,
  family,
  title,
  className,
}: ClientPillProps) {
  const name = (appName ?? '').trim() || (family ?? '').trim()
  if (!name) return null
  const icon = clientIconKey(name)
  return (
    <span
      className={cn(
        'inline-flex max-w-full items-center gap-1.5 rounded-full border bg-muted/40 py-0.5 pr-2.5 pl-1.5 text-xs font-medium',
        className
      )}
      title={title ?? name}
    >
      {icon ? (
        <BrandGlyph icon={icon} size={14} fallbackText={name} alt='' />
      ) : null}
      <span className='min-w-0 truncate'>{name}</span>
    </span>
  )
}
