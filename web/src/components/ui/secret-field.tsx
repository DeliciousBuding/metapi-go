// metapi-go/components/ui — reusable masked-secret display with reveal + copy.
// Shows the masked form by default; toggles to the full value on demand and
// offers one-click copy. Used for API keys/tokens where the raw value is
// sensitive but the URL/base_url is NOT (URLs stay plaintext elsewhere).

import { Check, Copy, Eye, EyeOff } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface SecretFieldProps {
  /** Full secret value (may be empty when only a masked form is available). */
  value?: string | null
  /** Masked display form (e.g. "sk-abc***xyz"). */
  masked?: string | null
  /** Text shown when neither value nor masked is present. */
  fallback?: string
  className?: string
}

export function SecretField({
  value,
  masked,
  fallback = '—',
  className,
}: SecretFieldProps) {
  const { t } = useTranslation()
  const [revealed, setRevealed] = useState(false)
  const [copied, setCopied] = useState(false)

  const full = value?.trim() || ''
  const mask = masked?.trim() || (full ? '••••••••••••' : fallback)
  const hasRevealable = full.length > 0
  const display = revealed && hasRevealable ? full : mask

  const handleCopy = async () => {
    const text = hasRevealable ? full : mask
    if (!text || text === fallback) return
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // Clipboard may be unavailable (non-secure context / permissions).
    }
  }

  return (
    <span
      className={cn('inline-flex max-w-full items-center gap-0.5', className)}
    >
      <span className='text-muted-foreground truncate font-mono text-[11px]'>
        {display}
      </span>
      {hasRevealable && (
        <Button
          variant='ghost'
          size='icon-sm'
          onClick={() => setRevealed((v) => !v)}
          title={revealed ? t('common.hide') : t('common.reveal')}
          aria-label={
            revealed ? t('common.hideSecret') : t('common.revealSecret')
          }
        >
          {revealed ? <EyeOff /> : <Eye />}
        </Button>
      )}
      <Button
        variant='ghost'
        size='icon-sm'
        onClick={handleCopy}
        title={t('common.copy')}
        aria-label={t('common.copySecret')}
      >
        {copied ? <Check className='text-success' /> : <Copy />}
      </Button>
    </span>
  )
}
