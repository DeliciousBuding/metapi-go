import { describe, expect, it } from 'vitest'

import { extractStaticPluginMetaPreview } from '../lib/plugin-meta-preview'

describe('extractStaticPluginMetaPreview', () => {
  it('extracts and validates an API v1 wire manifest without executing code', () => {
    const source = `
      // leading comment
      export const meta = {
        apiVersion: 1,
        key: "vendor-options",
        name: "Vendor options",
        version: "1.0.0",
        protocols: ["chat", "responses"],
        hooks: ["request"],
        options: [{ key: "mode", label: "Mode", type: "select", default: "safe", choices: ["safe", "fast"] }],
      };
      export function request(input) {
        throw new Error("must never execute during static preview");
      }
    `
    const result = extractStaticPluginMetaPreview(source, 'vendor-options')
    expect(result.status).toBe('valid')
    if (result.status !== 'valid') return
    expect(result.keyMismatch).toBe(false)
    expect(result.manifest.apiVersion).toBe(1)
    expect(result.manifest.protocols).toEqual(['chat', 'responses'])
  })

  it('extracts an API v2 provider manifest including custom auth header and endpoint paths', () => {
    const source = `
      export const meta = {
        apiVersion: 2,
        kind: "provider",
        key: "custom-provider",
        name: "Custom Provider",
        version: "2.1.0",
        protocols: ["chat"],
        hooks: ["request", "response", "modelsRequest", "modelsResponse"],
        credentialKinds: ["api_key"],
        authentication: {
          type: "api_key",
          header: "X-Custom-Auth",
          prefix: "Token ",
        },
        defaults: {
          endpoints: {
            chat: { path: "/v2/chat", auth: "bearer" },
          },
          catalog: { path: "/v2/models", auth: "bearer" },
        },
      };
    `
    const result = extractStaticPluginMetaPreview(source, 'other-key')
    expect(result.status).toBe('valid')
    if (result.status !== 'valid') return
    expect(result.keyMismatch).toBe(true)
    if (result.manifest.apiVersion !== 2) return
    expect(result.manifest.authentication?.header).toBe('X-Custom-Auth')
    expect(result.manifest.defaults?.endpoints?.chat?.path).toBe('/v2/chat')
  })

  it('rejects dynamic expressions without evaluating them', () => {
    const source = `
      const dyn = "oops";
      export const meta = {
        apiVersion: 1,
        key: dyn,
      };
    `
    const result = extractStaticPluginMetaPreview(source, 'dyn')
    expect(result.status).toBe('dynamic')
  })

  it('reports Zod schema issues on statically valid objects with invalid manifest fields', () => {
    const source = `
      export const meta = {
        apiVersion: 1,
        key: "bad-plugin",
        name: "Bad",
        version: "1.0.0",
        protocols: [],
        hooks: ["request"],
      };
    `
    const result = extractStaticPluginMetaPreview(source, 'bad-plugin')
    expect(result.status).toBe('invalid_schema')
  })
})
