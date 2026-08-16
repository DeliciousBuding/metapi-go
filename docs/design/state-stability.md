# UI State Stability Contract

This document is part of the MetAPI admin design system. Visual consistency is
not sufficient if a surface can enter a render/navigation feedback loop: state
ownership, callback identity, and browser responsiveness are UX contracts too.

## 1. Single owner for navigable state

List/table state that must survive refresh or be shareable by URL — global
search, pagination, sorting and faceted filters — is owned by the URL.

- Route `validateSearch` defines the canonical external contract.
- Feature pages decode/encode that contract with the shared
  `useUrlTableState` adapter.
- `useDataTable` consumes the resulting controlled state.
- Do **not** copy validated URL state into local `useState` and then write the
  local copy back to the router from `useEffect`. Two writable owners create a
  bidirectional synchronization loop and can snap state backward during route
  transitions.
- Filter and sort changes reset pagination in the **same URL update**. Do not
  emit a filter navigation followed by a second page-reset navigation.
- Serializers merge against the browser's latest URL at callback time when a
  table can emit multiple updates in one turn; one update must never erase an
  unrelated active filter.

Ephemeral UI state — open dialogs, selected detail rows, unsaved form values —
remains local unless it is intentionally part of a deep link.

## 2. Stable controlled callbacks

Callbacks passed into stateful primitives are part of the component contract,
not disposable render output.

- TanStack Table, Base UI and similar controlled primitives must receive stable
  state-change callback identities.
- Shared hooks use ref-backed callbacks when the latest caller behavior is
  needed without changing callback identity.
- Page actions that feed column definitions are memoized; avoid rebuilding a
  column/action graph on every render when no semantics changed.
- Never put a freshly-created object/array/function into an effect dependency
  chain that writes router state or parent-controlled state unless that identity
  change is the intended trigger.
- A callback may read the latest value through a ref, but render-visible values
  still come from React/router state; refs are not a second UI state store.

The failure mode this prevents is:

`render -> new callback identity -> controlled primitive effect -> state/router write -> render`

which can manifest as maximum-update-depth errors, route snap-back, a frozen
renderer, or a page that looks like a server failure even though the API is
healthy.

## 3. Navigation safety

URL synchronization must be a no-op when it cannot change the intended page.

- Guard route-specific synchronizers by canonical pathname.
- Suppress no-op history replacements when the target href equals the current
  href.
- Ignore stale controlled callbacks fired while the owning route is unmounting.
- Invalid/stale search params degrade to documented defaults rather than
  throwing a route-level error.
- Preserve the existing public URL convention (for example accounts keeps its
  historical 1-based `page` parameter even though TanStack uses a 0-based
  index internally).

## 4. Browser acceptance is a design-system gate

Unit tests cannot detect every renderer feedback loop. The shipped production
bundle therefore has two browser gates against the real embedded Go SPA:

- `bun run a11y:axe` — axe-core serious/critical accessibility scan.
- `bun run ui:smoke` — real Chromium route/responsiveness scan over desktop and
  mobile surfaces plus high-risk interactions.
- `bun run a11y:scan` — composite CI-facing browser gate; runs both commands.

`ui:smoke` fails on uncaught page errors, `console.error`, failed requests, HTTP
5xx responses, known React fatal/error-boundary text, renderer non-yield, and
mobile document-level horizontal overflow. High-risk regressions should add a
small interaction to this gate rather than relying only on route load.

## 5. Review checklist

Before adding a URL-synced or controlled surface, verify:

1. There is exactly one writable owner for every piece of persistent state.
2. Controlled callbacks are referentially stable across unrelated renders.
3. A state change produces at most one canonical navigation transaction.
4. The serializer preserves unrelated active URL state.
5. Route unmount cannot write stale state back into another page.
6. The page remains responsive in production Chromium at desktop and mobile
   widths.
7. A regression test covers any interaction that previously caused a loop,
   crash, or route-level error.
