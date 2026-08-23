// metapi-go/ui — sheet component ported from newapi (base-nova style, @base-ui/react). AGPL header stripped.
'use client'

import { Dialog as SheetPrimitive } from '@base-ui/react/dialog'
import { Cancel01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import * as React from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

function Sheet({ ...props }: SheetPrimitive.Root.Props) {
  return <SheetPrimitive.Root data-slot='sheet' {...props} />
}

function SheetPortal({ ...props }: SheetPrimitive.Portal.Props) {
  return <SheetPrimitive.Portal data-slot='sheet-portal' {...props} />
}

function SheetOverlay({ className, ...props }: SheetPrimitive.Backdrop.Props) {
  return (
    <SheetPrimitive.Backdrop
      data-slot='sheet-overlay'
      className={cn(
        'motion-reduce:animate-none! data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0 fixed inset-0 z-50 bg-overlay duration-150 supports-backdrop-filter:backdrop-blur-xs',
        className
      )}
      {...props}
    />
  )
}

function SheetContent({
  className,
  children,
  side = 'right',
  showCloseButton = true,
  overlayClassName,
  ...props
}: SheetPrimitive.Popup.Props & {
  side?: 'top' | 'right' | 'bottom' | 'left'
  showCloseButton?: boolean
  /** Extra classes for the scrim (e.g. to darken a drawer's backdrop). */
  overlayClassName?: string
}) {
  const { t } = useTranslation()
  // Side-specific classes are emitted via JS conditionals (rather than
  // `data-[side=*]:` variants) so consumer-provided width overrides such as
  // `sm:max-w-2xl` can be correctly merged by `tailwind-merge` and the CSS
  // cascade — the data-attribute variants would otherwise win on specificity
  // and trap the panel at the default `sm:max-w-sm` width.
  //
  // Small-screen contract (≤640px): right/left panels are full-width
  // (`w-full`) and only narrow to `sm:w-3/4` (capped by `sm:max-w-sm`) at
  // `sm+`, so a 375px viewport gets an edge-to-edge panel. The panel itself
  // is the scroll container (`overflow-y-auto`) so tall content scrolls
  // instead of being clipped; consumers that manage their own scroll region
  // (e.g. a `flex-1 overflow-y-auto` body with a sticky footer) simply never
  // overflow the panel. Desktop sizing/behavior is unchanged.
  return (
    <SheetPortal>
      <SheetOverlay className={overlayClassName} />
      <SheetPrimitive.Popup
        data-slot='sheet-content'
        data-side={side}
        className={cn(
          'bg-background text-foreground motion-reduce:animate-none! data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0 fixed z-50 flex flex-col gap-4 overflow-y-auto bg-clip-padding text-sm shadow-none duration-200',
          side === 'right' &&
            'inset-y-0 right-0 h-full w-full border-l data-open:slide-in-from-right data-closed:slide-out-to-right sm:w-3/4 sm:max-w-sm',
          side === 'left' &&
            'inset-y-0 left-0 h-full w-full border-r data-open:slide-in-from-left data-closed:slide-out-to-left sm:w-3/4 sm:max-w-sm',
          side === 'top' &&
            'inset-x-0 top-0 max-h-full h-auto border-b data-open:slide-in-from-top data-closed:slide-out-to-top',
          side === 'bottom' &&
            'inset-x-0 bottom-0 max-h-full h-auto border-t data-open:slide-in-from-bottom data-closed:slide-out-to-bottom',
          className
        )}
        {...props}
      >
        {children}
        {showCloseButton && (
          <SheetPrimitive.Close
            data-slot='sheet-close'
            render={
              <Button
                variant='ghost'
                className='absolute top-3 right-3'
                size='icon-sm'
              />
            }
          >
            <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
            <span className='sr-only'>{t('common.close')}</span>
          </SheetPrimitive.Close>
        )}
      </SheetPrimitive.Popup>
    </SheetPortal>
  )
}

function SheetHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='sheet-header'
      className={cn('flex flex-col gap-0.5 p-4', className)}
      {...props}
    />
  )
}

function SheetFooter({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='sheet-footer'
      className={cn('mt-auto flex flex-col gap-2 p-4', className)}
      {...props}
    />
  )
}

function SheetTitle({ className, ...props }: SheetPrimitive.Title.Props) {
  return (
    <SheetPrimitive.Title
      data-slot='sheet-title'
      className={cn('text-foreground text-base font-medium', className)}
      {...props}
    />
  )
}

function SheetDescription({
  className,
  ...props
}: SheetPrimitive.Description.Props) {
  return (
    <SheetPrimitive.Description
      data-slot='sheet-description'
      className={cn('text-muted-foreground text-sm', className)}
      {...props}
    />
  )
}

export {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetFooter,
  SheetTitle,
  SheetDescription,
}
