// metapi-go/ui — switch component ported from newapi (base-nova style, @base-ui/react). AGPL header stripped.
import { Switch as SwitchPrimitive } from '@base-ui/react/switch'
import { useEffect, useRef } from 'react'

import { cn } from '@/lib/utils'

function Switch({
  className,
  size = 'default',
  ...props
}: SwitchPrimitive.Root.Props & {
  size?: 'sm' | 'default'
}) {
  const ref = useRef<HTMLElement | null>(null)

  // @base-ui emits aria-checked as "0"/"1", but ARIA only accepts
  // "true"/"false"/"mixed"/"undefined" — normalize so axe and screen
  // readers see a valid value (kept in sync on every re-render).
  useEffect(() => {
    const node = ref.current
    if (!node) return
    const current = node.getAttribute('aria-checked')
    if (current === '0') node.setAttribute('aria-checked', 'false')
    else if (current === '1') node.setAttribute('aria-checked', 'true')
  })

  return (
    <SwitchPrimitive.Root
      ref={ref}
      data-slot='switch'
      data-size={size}
      className={cn(
        'peer group/switch focus-visible:border-ring focus-visible:ring-ring/50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40 data-checked:bg-primary data-unchecked:bg-input dark:data-unchecked:bg-input/80 relative inline-flex shrink-0 items-center rounded-full border border-transparent transition-colors outline-none after:absolute after:-inset-x-3 after:-inset-y-2 focus-visible:ring-3 aria-invalid:ring-3 data-disabled:cursor-not-allowed data-disabled:opacity-50 data-[size=default]:h-[18.4px] data-[size=default]:w-[32px] data-[size=sm]:h-[14px] data-[size=sm]:w-[24px]',
        className
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb
        data-slot='switch-thumb'
        className='bg-background dark:data-checked:bg-primary-foreground dark:data-unchecked:bg-foreground pointer-events-none block rounded-full ring-0 transition-transform group-data-[size=default]/switch:size-4 group-data-[size=sm]/switch:size-3 group-data-[size=default]/switch:data-checked:translate-x-[calc(100%-2px)] group-data-[size=sm]/switch:data-checked:translate-x-[calc(100%-2px)] group-data-[size=default]/switch:data-unchecked:translate-x-0 group-data-[size=sm]/switch:data-unchecked:translate-x-0'
      />
    </SwitchPrimitive.Root>
  )
}

export { Switch }
