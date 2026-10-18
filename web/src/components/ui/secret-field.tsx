// metapi-go/components/ui — reusable masked-secret display with reveal + copy.
// Shows the masked form by default; toggles to the full value on demand and
// offers one-click copy. Used for API keys/tokens where the raw value is
// sensitive but the URL/base_url is NOT (URLs stay plaintext elsewhere).
//
// The full value may be unavailable until an explicit, gated read succeeds
// (e.g. an upstream token behind master-token confirmation). Callers pass
// onRevealRequest/onCopyRequest and supply `value` once the read returns; the
// component never fetches credentials itself.

import {
  CheckIcon,
  Copy01Icon,
  EyeIcon,
  EyeOffIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { copyText } from '@/lib/clipboard'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'

interface SecretFieldProps {
  /** Full secret value (may be empty when only a masked form is available). */
  value?: string | null
  /** Masked display form (e.g. "sk-abc***xyz"). */
  masked?: string | null
  /** Text shown when neither value nor masked is present. */
  fallback?: string
  className?: string
  /** Ask the owner to load the plaintext before revealing; omitted when `value` is already present. */
  onRevealRequest?: () => void
  /** Ask the owner to load the plaintext before copying; omitted when `value` is already present. */
  onCopyRequest?: () => void
  /** Disable both controls while an owner request is in flight. */
  busy?: boolean
}

export function SecretField({
  value,
  masked,
  fallback = '—',
  className,
  onRevealRequest,
  onCopyRequest,
  busy = false,
}: SecretFieldProps) {
  const { t } = useTranslation()
  const [revealed, setRevealed] = useState(false)
  const [copied, setCopied] = useState(false)

  const full = value?.trim() || ''
  const mask = masked?.trim() || (full ? '••••••••••••' : fallback)
  const canReveal = full.length > 0 || !!onRevealRequest
  const display = revealed && full ? full : mask

  const handleReveal = () => {
    if (!full) {
      if (onRevealRequest) {
        setRevealed(true)
        onRevealRequest()
      }
      return
    }
    setRevealed((v) => !v)
  }

  const handleCopy = async () => {
    const text = full || mask
    if (!text || text === fallback) return
    if (!full && onCopyRequest) {
      onCopyRequest()
      return
    }
    if (await copyText(text)) {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } else {
      // Clipboard may be unavailable (non-secure context / permissions).
      toast.error(t('common.copyFailed'))
    }
  }

  return (
    <span
      className={cn('inline-flex max-w-full items-center gap-0.5', className)}
    >
      <span className='text-muted-foreground text-2xs truncate font-mono'>
        {display}
      </span>
      {canReveal && (
        <Button
          variant='ghost'
          size='icon-sm'
          disabled={busy}
          onClick={handleReveal}
          title={revealed ? t('common.hide') : t('common.reveal')}
          aria-label={
            revealed ? t('common.hideSecret') : t('common.revealSecret')
          }
        >
          {revealed ? (
            <HugeiconsIcon icon={EyeOffIcon} strokeWidth={2} />
          ) : (
            <HugeiconsIcon icon={EyeIcon} strokeWidth={2} />
          )}
        </Button>
      )}
      <Button
        variant='ghost'
        size='icon-sm'
        disabled={busy}
        onClick={handleCopy}
        title={t('common.copy')}
        aria-label={t('common.copySecret')}
      >
        {copied ? (
          <HugeiconsIcon
            icon={CheckIcon}
            strokeWidth={2}
            className='text-success'
          />
        ) : (
          <HugeiconsIcon icon={Copy01Icon} strokeWidth={2} />
        )}
      </Button>
    </span>
  )
}
