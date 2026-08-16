// Package pricingcatalog serves official catalog pricing from models.dev as a
// cold-start cost signal for the token router.
//
// The catalog is the models.dev community dataset (https://models.dev/api.json):
// official per-million-token list prices per provider/model. It is refreshed
// periodically and parsed into an in-memory map; when a fetch fails the
// provider keeps the previous snapshot, or falls back to a small compile-time
// preset table when nothing has been fetched yet.
//
// Provenance honesty: catalog prices are official vendor list prices. They are
// labeled "catalog" only when the site points at the vendor's own API host. For
// third-party relay sites the official list price is at best an estimate of
// what the relay actually charges, so those queries are labeled
// "catalog_estimate" (mirroring routing.CatalogSourceRelayEstimate) and must
// never be presented as a real payment price.
package pricingcatalog
