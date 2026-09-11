// metapi-go/ui — DropdownMenuItem event bridge.
//
// Base UI's Menu.Item exposes a single click handler, but metapi-go dropdown
// items offer the familiar onClick + onSelect pair. This module owns that one
// translation so DropdownMenuItem stays declarative:
//   1. onClick always runs;
//   2. onSelect runs only if onClick did not cancel the event;
//   3. if the event ends up cancelled (by either handler), tell Base UI to keep
//      the menu open via preventBaseUIHandler.

import type * as React from 'react'

/** A menu-item click, extended with Base UI's "don't close the menu" escape hatch. */
export type DropdownMenuItemSelectEvent = React.MouseEvent<HTMLElement> & {
  preventBaseUIHandler?: () => void
}

/** Handler for a menu item's select event (see DropdownMenuItemSelectEvent). */
export type DropdownMenuItemSelectHandler = (
  event: DropdownMenuItemSelectEvent
) => void

/**
 * Bridge a Base UI Menu.Item click onto the onClick + onSelect contract,
 * honoring preventDefault from either handler.
 */
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
