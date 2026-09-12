// metapi-go/features/downstream-keys — public barrel.
//
// Consumers should import only from here:
//   import {
//     downstreamKeysQueryKeys,
//     type DownstreamKeysResponse,
//   } from '@/features/downstream-keys'
//
// What this barrel owns is the wire contract other features read. The section
// UI still lives under `features/settings/sections/downstream/` and the route
// file loads `./downstream-keys-page` directly: re-exporting that page here
// would make the barrel pull in a module that lazy-imports the settings
// section, which in turn imports this barrel.

export { downstreamKeysQueryKeys } from './types'
export type {
  CreateDownstreamKeyResponse,
  DownstreamApiKeyItem,
  DownstreamKeysResponse,
} from './types'
