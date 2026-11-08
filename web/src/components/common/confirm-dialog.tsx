// metapi-go/components/common — controlled confirmation dialog built on
// the project AlertDialog. Shared by the settings navigation guard, the
// import confirm step, the schedule migration card, and the feature forms.

import { useEffect, useId, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Input } from '@/components/ui/input'

type ConfirmDialogProps = {
  open: boolean
  title: string
  description?: string
  confirmLabel: string
  cancelLabel: string
  destructive?: boolean
  confirmationWord?: string
  isPending?: boolean
  confirmDisabled?: boolean
  finalFocus?: () => HTMLElement | null
  children?: ReactNode
  onConfirm: () => void
  onCancel: () => void
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  cancelLabel,
  destructive,
  confirmationWord,
  isPending,
  confirmDisabled,
  finalFocus,
  children,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  if (confirmationWord) {
    return open ? (
      <TypedConfirmDialog
        key={confirmationWord}
        open
        title={title}
        description={description}
        confirmLabel={confirmLabel}
        cancelLabel={cancelLabel}
        destructive={destructive}
        confirmationWord={confirmationWord}
        isPending={isPending}
        confirmDisabled={confirmDisabled}
        finalFocus={finalFocus}
        onConfirm={onConfirm}
        onCancel={onCancel}
      >
        {children}
      </TypedConfirmDialog>
    ) : null
  }
  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          onCancel()
        }
      }}
    >
      <AlertDialogContent finalFocus={finalFocus}>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description ? (
            <AlertDialogDescription>{description}</AlertDialogDescription>
          ) : null}
        </AlertDialogHeader>
        {children}
        <AlertDialogFooter>
          <AlertDialogCancel onClick={onCancel}>
            {cancelLabel}
          </AlertDialogCancel>
          <AlertDialogAction
            variant={destructive ? 'destructive' : undefined}
            disabled={confirmDisabled || isPending}
            onClick={onConfirm}
          >
            {confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

// Irreversible operations use the design system's countdown + typed-word tier.
// This child unmounts on close, so reopening always requires a fresh review.
function TypedConfirmDialog(
  props: ConfirmDialogProps & { confirmationWord: string }
) {
  const { t } = useTranslation()
  const inputId = useId()
  const [countdown, setCountdown] = useState(3)
  const [word, setWord] = useState('')
  useEffect(() => {
    if (countdown === 0) return
    const timer = window.setTimeout(
      () => setCountdown((remaining) => remaining - 1),
      1000
    )
    return () => window.clearTimeout(timer)
  }, [countdown])
  const canConfirm =
    countdown === 0 &&
    word.trim() === props.confirmationWord &&
    !props.isPending &&
    !props.confirmDisabled
  return (
    <AlertDialog
      open
      onOpenChange={(next) => {
        if (!next && !props.isPending) props.onCancel()
      }}
    >
      <AlertDialogContent finalFocus={props.finalFocus}>
        <AlertDialogHeader>
          <AlertDialogTitle>{props.title}</AlertDialogTitle>
          {props.description && (
            <AlertDialogDescription className='min-w-0 wrap-anywhere'>
              {props.description}
            </AlertDialogDescription>
          )}
        </AlertDialogHeader>
        {props.children}
        <div className='grid gap-2'>
          <label htmlFor={inputId} className='text-sm'>
            {t('common.upstreamDeletion.typeHint', {
              word: props.confirmationWord,
            })}
          </label>
          <Input
            id={inputId}
            value={word}
            onChange={(event) => setWord(event.target.value)}
            disabled={props.isPending}
            autoComplete='off'
            spellCheck={false}
            placeholder={props.confirmationWord}
          />
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={props.isPending}>
            {props.cancelLabel}
          </AlertDialogCancel>
          <AlertDialogAction
            variant={props.destructive ? 'destructive' : undefined}
            disabled={!canConfirm}
            onClick={() => {
              if (canConfirm) props.onConfirm()
            }}
          >
            {countdown > 0
              ? t('common.upstreamDeletion.countdown', { seconds: countdown })
              : props.confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
