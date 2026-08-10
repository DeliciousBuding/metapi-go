// metapi-go/ui — dropdown-menu event helper (ported from newapi, AGPL header stripped).
// Bridges Base UI Menu.Item onClick + onSelect semantics: onSelect fires only
// when the click isn't defaultPrevented, and preventBaseUIHandler stops the
// menu from closing when a handler calls preventDefault.

import type * as React from 'react'

export type DropdownMenuItemSelectEvent = React.MouseEvent<HTMLElement> & {
  preventBaseUIHandler?: () => void
}

export type DropdownMenuItemSelectHandler = (
  event: DropdownMenuItemSelectEvent
) => void

export function handleDropdownMenuItemSelect(
  event: DropdownMenuItemSelectEvent,
  onClick?: React.MouseEventHandler<HTMLElement>,
  onSelect?: DropdownMenuItemSelectHandler
) {
  onClick?.(event)

  if (!event.defaultPrevented) {
    onSelect?.(event)
  }

  if (event.defaultPrevented) {
    event.preventBaseUIHandler?.()
  }
}
