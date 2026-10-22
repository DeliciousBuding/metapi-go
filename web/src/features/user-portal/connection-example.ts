import type { PersonalModel } from './api'

// Values in a public model name are still untrusted shell input. Keep the
// credential as an environment reference, outside URLs, snippets and caches.
function shellQuote(value: string): string {
  return "'" + value.replaceAll("'", "'\\''") + "'"
}

export function connectionExample(
  origin: string,
  model: string,
  protocol: PersonalModel['protocols'][number]
): string {
  const body =
    protocol === 'embeddings'
      ? { model, input: 'Hello' }
      : protocol === 'responses'
        ? { model, input: 'Hello', max_output_tokens: 64 }
        : {
            model,
            messages: [{ role: 'user', content: 'Hello' }],
            max_tokens: 64,
          }
  const headers =
    protocol === 'messages'
      ? [
          '  -H "x-api-key: ${METAPI_API_KEY:?Set METAPI_API_KEY first}"',
          "  -H 'anthropic-version: 2023-06-01'",
        ]
      : [
          '  -H "Authorization: Bearer ${METAPI_API_KEY:?Set METAPI_API_KEY first}"',
        ]
  return [
    'curl --include --request POST ' + shellQuote(origin + '/v1/' + protocol),
    ...headers,
    "  -H 'Content-Type: application/json'",
    '  --data-raw ' + shellQuote(JSON.stringify(body, null, 2)),
  ].join(' \\\n')
}
