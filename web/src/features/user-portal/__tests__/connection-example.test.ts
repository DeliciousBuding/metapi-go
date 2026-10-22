import { execFileSync } from 'node:child_process'

import { describe, expect, it } from 'vitest'

import { connectionExample } from '../connection-example'

describe('personal connection examples', () => {
  it.each(['chat/completions', 'responses', 'messages', 'embeddings'] as const)(
    'produces an executable %s request without embedding credentials',
    (protocol) => {
      const model = "public-'$(printf injected)`printf injected`\nmodel"
      const example = connectionExample(
        'https://gateway.example',
        model,
        protocol
      )
      const args = execFileSync(
        'sh',
        ['-c', 'curl() { printf "%s\\000" "$@"; }; ' + example],
        { env: { ...process.env, METAPI_API_KEY: 'synthetic-personal-key' } }
      )
        .toString()
        .split('\0')
      expect(args).toContain('https://gateway.example/v1/' + protocol)
      expect(args).toContain(
        protocol === 'messages'
          ? 'x-api-key: synthetic-personal-key'
          : 'Authorization: Bearer synthetic-personal-key'
      )
      const body = JSON.parse(args[args.indexOf('--data-raw') + 1]!)
      expect(body.model).toBe(model)
      if (protocol === 'embeddings')
        expect(body).toEqual({ model, input: 'Hello' })
      else if (protocol === 'responses') expect(body.max_output_tokens).toBe(64)
      else expect(body.max_tokens).toBe(64)
      expect(example).not.toContain('synthetic-personal-key')
    }
  )
  it('refuses to invoke curl without an explicitly configured personal key', () => {
    expect(() =>
      execFileSync(
        'sh',
        [
          '-c',
          'unset METAPI_API_KEY; curl() { printf should-not-run; }; ' +
            connectionExample('https://gateway.example', 'model', 'responses'),
        ],
        { stdio: 'pipe' }
      )
    ).toThrow()
  })
})
