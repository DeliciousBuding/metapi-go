// metapi-go/lib — shared responsive breakpoint constants.
//
// Before this module existed, the table→card-list switch (640px) and the
// sidebar→drawer switch (768px) were independent magic strings buried in
// `data-table-page.tsx` and `use-mobile.tsx`. These constants are the single
// source of truth so both call sites (and their tests) stay in sync.

/**
 * Viewport width (px) at and below which `DataTablePage` swaps the desktop
 * table for the `MobileCardList` rendering. Matches the `max-sm`/`sm`
 * Tailwind boundary used by the card-list styling.
 */
export const TABLE_MOBILE_MAX_WIDTH = 640

/**
 * Viewport width (px) at and below which the app chrome switches to the
 * mobile layout (sidebar becomes a drawer, header hamburger). One less than
 * the 768px `md` boundary so a 768px viewport stays classified as desktop —
 * mirroring the original `innerWidth < 768` check.
 */
export const SIDEBAR_MOBILE_MAX_WIDTH = 767

/** Media query for the table→card-list switch (`DataTablePage`). */
export const TABLE_MOBILE_MEDIA_QUERY = `(max-width: ${TABLE_MOBILE_MAX_WIDTH}px)`

/** Media query for the sidebar→drawer switch (`useIsMobile`). */
export const SIDEBAR_MOBILE_MEDIA_QUERY = `(max-width: ${SIDEBAR_MOBILE_MAX_WIDTH}px)`

/*
 * The 641–767px band between the two thresholds is intentional, not a gap:
 * navigation already uses the mobile drawer (sidebar collapsed) while tables
 * keep the desktop layout with horizontal scrolling inside their bordered
 * container. Two different concerns, two different thresholds — do not
 * "unify" them without a product decision.
 */
