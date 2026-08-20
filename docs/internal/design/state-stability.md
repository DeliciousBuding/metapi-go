# State stability — URL-owned table/filter state

**Scope**: how list pages own their navigable state (search / page / page-size /
sorting / faceted + page-specific filters) without falling into render loops,
URL write-back loops, or back/forward corruption.

**Source of truth for the rules below**: `web/src/components/data-table/hooks/use-url-table-state.ts`
and `web/src/components/data-table/hooks/use-data-table.ts`. The list pages
(sites / models / oauth / channels / accounts / checkin / proxy-logs /
token-routes) all build on these two hooks.

---

## 1. The bug this prevents

The accounts page historically froze the renderer. The root cause was a
feedback loop, not backend load:

```
URL state
  ↓
React controlled state (useState mirror)
  ↓
TanStack Table
  ↓
onChange callback
  ↓
router navigate
  ↓
URL state
```

Three concrete triggers, all now banned:

1. **Unstable callback identity** — a fresh inline `onPaginationChange` /
   `onGlobalFilterChange` / `onColumnFiltersChange` every render re-resolves the
   TanStack table, which re-runs its `autoResetPageIndex` effect, which can feed
   back through the URL sync into an infinite loop.
2. **URL owned by local state + an effect** — `useState(initial)` seeded from the
   URL plus a `useEffect(() => navigate(...))` write-back creates a two-way sync
   where either side can fight the other.
3. **Multiple navigations per logical change** — changing a filter resets the
   page in a *separate* navigate, so an older transaction can overwrite a newer
   one.

## 2. Rules

### R1 — the URL is the single owner of persistent navigable state

For a table page, the URL search string is the **only** source of truth for
search / page / page-size / sorting / filters. There is no `useState` mirror of
any of these values. Pages **derive** the current view from the URL and **write**
changes straight back to the URL.

```text
Browser URL
  → route schema / parser (`read`)
  → useUrlTableState (derived values + stable callbacks)
  → useDataTable / controls
```

User input:

```text
UI action → stable callback → ONE logical URL update → router → UI re-derives
```

The banned shape is:

```text
URL ⇄ useState mirror ⇄ table ⇄ useEffect ⇄ URL
```

### R2 — controlled callbacks must keep a stable identity

`useDataTable` hands the controlled-state callbacks directly into
`useReactTable`. If their identity changes every render, TanStack re-resolves
the table. Therefore:

- `useUrlTableState` returns `onGlobalFilterChange` / `onPaginationChange` /
  `onSortingChange` / `onColumnFiltersChange` as `useCallback`s whose deps are
  the parsed URL state (which only changes on navigation).
- `useControllableTableState` inside `useDataTable` keeps its `setValue` stable
  by reading the `onChange` prop through a ref (`onChangeRef`).
- Page-supplied option functions (`read` / `buildHref` / `toColumnFilters` /
  `fromColumnFilters`) are read through refs so inline definitions do not break
  the contract.
- Table `globalFilterFn` and `rowActions`/`columnActions` live at module level or
  in `useMemo`, never inline per render.

### R3 — one logical change = one URL transaction

A filter change that also resets the page must be a single `navigate`, not
`navigate(filters)` followed by `navigate(page=1)`.

- Server-side pages pass `resetPageIndexOnFilterChange: true` to
  `useUrlTableState`, which folds `pageIndex: 0` into the same update as the
  filter change, and pass `autoResetPageIndex: false` to `useDataTable` so
  TanStack does not fire a second, redundant reset.
- Page-specific controls (date ranges, account/client selects, latency bounds)
  call `updateUrlState({ filters: { … }, pageIndex: 0 })` — one transaction.

### R4 — URL updates merge the *latest* URL

Serializers must merge over the current `window.location.search`, never over a
stale closure captured during render. Every page `buildHref` reads the current
URL, spreads the partial update over it, and shallow-merges `filters`:

```ts
const current = readSearch(window.location.search)
const merged = { ...current, ...next, filters: { ...current.filters, ...next.filters } }
```

This is why changing `q` cannot accidentally drop an existing `status`/`site`
filter.

### R5 — guard pathname, unmount, and same-href

`updateUrlState` ignores a change when:

- the built href is for a different pathname (the user navigated away and a
  stale table callback fired — the "clicked a sidebar link and snapped back"
  bug), or
- the built href equals the current href (no-op, no navigation).

### R6 — browser acceptance is part of the design system

`typecheck + lint + test` cannot prove a UI does not freeze or that back/forward
restores the right view. A real-Chromium smoke/axe gate is mandatory for these
pages (`web/scripts/route-smoke.mjs` + `web/scripts/a11y-scan.mjs`), and is run
in CI against the real production bundle.

---

## 3. The shared hooks

### `useUrlTableState<TFilters>`

Per-page contract (implemented via `read` / `buildHref` / `toColumnFilters` /
`fromColumnFilters`):

| Returned value | What it is |
|----------------|-----------|
| `globalFilter` / `pagination` / `sorting` / `columnFilters` | derived from the URL; plug into `useDataTable` |
| `onGlobalFilterChange` / `onPaginationChange` / `onSortingChange` / `onColumnFiltersChange` | stable callbacks that navigate |
| `filters` | the raw page-specific `TFilters` (for controls + server query payloads) |
| `updateUrlState(next)` | generic single-transaction navigate for page-specific filters |
| `ensurePageInRange(pageCount)` | clamp the URL page when data shrinks |

`UrlTableStateUpdate<TFilters>` is a partial update where `filters` is itself
`Partial<TFilters>`, so a caller changes only the fields it owns.

### `useDataTable`

Owns the TanStack plumbing (row models, column visibility/sizing persistence,
selection, pagination). Its `useControllableTableState` guarantees a stable
setter identity for the controlled states it manages.

---

## 4. Page checklist

When adding or auditing a list page:

1. There is **no** `useState` initialized from `useSearch`/`window.location.search`
   for any URL-backed field.
2. There is **no** `useEffect` that calls `navigate` to write state back.
3. Every table callback is a stable `useUrlTableState` return (not inline).
4. Filter + page reset happen in **one** `updateUrlState`/`navigate`.
5. `buildHref` merges the current URL (never a stale closure).
6. `globalFilterFn` / `rowActions` / `columnActions` are module-level or memoized.
7. Server-side pages use `resetPageIndexOnFilterChange: true` +
   `autoResetPageIndex: false`.
8. Deep links and back/forward round-trip (verified by the browser smoke gate).

## 5. Anti-patterns (banned)

- `useState(initialSearch)` + `useEffect(() => navigate({ search, replace: true }))`.
- `history.replaceState` / `window.history.replaceState` called directly by a page
  (always go through the router).
- Inline `onPaginationChange={(value) => …}` / `globalFilterFn={(row) => …}`.
- Multiple `navigate` calls for one user action (e.g. filter then page reset).
- Serializing from a render-time snapshot of `search` instead of the live URL.
