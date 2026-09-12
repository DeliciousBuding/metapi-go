// metapi-go/features/downstream-keys — public barrel.
//
// Consumers should import only from here:
//   import {
//     downstreamKeysQueryKeys,
//     type DownstreamKeysResponse,
//   } from '@/features/downstream-keys'
//
// What this barrel publishes is the slice of the wire contract other features
// read: `dashboard`'s onboarding checklist and `token-routes`' next-step strip
// both query the key list and nothing else. The rest of `./types` is this
// feature's own business and stays out of it — the CI `knip` step fails on an
// export with no consumer, so this list is the measured surface, not a guess.
// Export from here when a second feature actually needs the symbol.
//
// The keys UI in `./components/` is deliberately NOT re-exported either: the
// route file loads `./downstream-keys-page` directly (route files are the
// composition root), and a barrel that exported the page would pull the whole
// list + sheet form into every consumer's chunk.

export { downstreamKeysQueryKeys } from './types'
export type { DownstreamKeysResponse } from './types'
