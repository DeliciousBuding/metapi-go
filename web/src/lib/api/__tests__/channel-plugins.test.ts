import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  channelPluginsApi,
  pluginBindingSchema,
  providerBindingSchema,
} from '../channel-plugins'

const mock = vi.hoisted(() => vi.fn())
vi.mock('../transport', () => ({ request: mock }))
beforeEach(() => mock.mockReset())

describe('channel plugin API data boundary', () => {
  it('preserves explicit asynchronous image slots and requires their full lifecycle', async () => {
    const protocols = [
      'imageGeneration',
      'imageEdit',
      'imageVariation',
      'modelscopeImageGeneration',
    ]
    const manifest = {
      apiVersion: 2,
      kind: 'provider',
      key: 'images',
      name: 'Image tasks',
      version: '1.0.0',
      protocols,
      credentialKinds: ['api_key'],
      hooks: ['submit', 'query', 'taskResult'],
      imageTasks: Object.fromEntries(
        protocols.map((protocol, index) => [
          protocol,
          {
            query: {
              path: `/tasks/${index}/{taskId}`,
              auth: ['bearer', 'x-api-key', 'x-goog-api-key', 'none'][index],
            },
          },
        ])
      ),
      observations: Object.fromEntries(
        protocols.map((protocol) => [
          protocol,
          { response: { taskIdPath: '/task/id' } },
        ])
      ),
    }
    const item = (value: unknown) => ({
      items: [{ key: 'images', enabled: true, revision: 1, manifest: value }],
    })
    for (const valid of [
      manifest,
      {
        ...manifest,
        imageTasks: {
          ...manifest.imageTasks,
          imageGeneration: {
            ...manifest.imageTasks.imageGeneration,
            stream: true,
          },
          imageEdit: { ...manifest.imageTasks.imageEdit, stream: true },
          modelscopeImageGeneration: {
            ...manifest.imageTasks.modelscopeImageGeneration,
            stream: false,
          },
        },
      },
      {
        ...manifest,
        protocols: [...protocols, 'chat'],
        hooks: [...manifest.hooks, 'request', 'response'],
      },
    ]) {
      mock.mockResolvedValueOnce(item(valid))
      await expect(channelPluginsApi.list()).resolves.toEqual(item(valid))
    }
    for (const invalid of [
      {
        ...manifest,
        imageTasks: {
          ...manifest.imageTasks,
          imageVariation: {
            ...manifest.imageTasks.imageVariation,
            stream: true,
          },
        },
      },
      {
        ...manifest,
        imageTasks: {
          ...manifest.imageTasks,
          imageGeneration: {
            ...manifest.imageTasks.imageGeneration,
            stream: 'true',
          },
        },
      },
      { ...manifest, imageTasks: {} },
      { ...manifest, imageTasks: null },
      { ...manifest, observations: undefined },
      { ...manifest, protocols: protocols.slice(1) },
      { ...manifest, hooks: ['request', 'response'] },
      ...manifest.hooks.map((missing) => ({
        ...manifest,
        hooks: manifest.hooks.filter((hook) => hook !== missing),
      })),
      {
        ...manifest,
        imageTasks: {
          chat: { query: { path: '/tasks/{taskId}', auth: 'none' } },
        },
      },
      {
        ...manifest,
        apiVersion: 1,
        kind: undefined,
        credentialKinds: undefined,
      },
    ]) {
      mock.mockResolvedValueOnce(item(invalid))
      await expect(channelPluginsApi.list()).rejects.toThrow()
    }
  })
  it('rejects unsafe image task templates without normalizing away the bad path', async () => {
    const manifest = {
      apiVersion: 2,
      kind: 'provider',
      key: 'images',
      name: 'Image tasks',
      version: '1',
      protocols: ['imageGeneration'],
      credentialKinds: ['none'],
      hooks: ['submit', 'query', 'taskResult'],
      observations: { imageGeneration: { response: { taskIdPath: '/id' } } },
    }
    const item = (query: unknown) => ({
      items: [
        {
          key: 'images',
          enabled: true,
          revision: 1,
          manifest: { ...manifest, imageTasks: { imageGeneration: { query } } },
        },
      ],
    })
    for (const path of [
      '/tasks',
      '/tasks/{taskId}/{taskId}',
      '/tasks/{taskId}suffix',
      '/tasks/{other}',
      '//other.invalid/{taskId}',
      '/tasks/../{taskId}',
      '/tasks/%2e%2e/{taskId}',
      '/tasks/{taskId}?key=fixture',
      '/tasks/{taskId}#fragment',
      '/tasks/%3f/{taskId}',
      '/tasks/%5c/{taskId}',
      '/tasks/%0a/{taskId}',
      '/tasks/\u0085/{taskId}',
      '/tasks/{taskId}/%',
      `/tasks/${'图'.repeat(2800)}/{taskId}`,
    ]) {
      mock.mockResolvedValueOnce(item({ path, auth: 'none' }))
      await expect(channelPluginsApi.list()).rejects.toThrow()
    }
    for (const query of [
      { path: '/tasks/{taskId}', auth: 'cookie' },
      { path: '/tasks/{taskId}', auth: 'none', modelPath: true },
      { path: '/tasks/{taskId}', auth: 'none', profile: 'modelscope' },
      { path: '/tasks/{taskId}', auth: 'none', headers: {} },
    ]) {
      mock.mockResolvedValueOnce(item(query))
      await expect(channelPluginsApi.list()).rejects.toThrow()
    }
  })
  it('round-trips saved per-slot image queries and rejects malformed or unselected targets', () => {
    const binding = {
      key: 'images',
      protocols: ['imageGeneration', 'imageEdit'],
      imageQueries: {
        imageGeneration: {
          url: 'https://vendor.invalid/tasks/{taskId}',
          auth: 'none',
        },
        imageEdit: {
          url: 'https://other.invalid/edits/{taskId}',
          auth: 'x-api-key',
        },
      },
    }
    expect(providerBindingSchema.parse(binding)).toEqual(binding)
    const unicodeUrl = `https://vendor.invalid/tasks/${'图'.repeat(2700)}/{taskId}`
    expect(
      providerBindingSchema.safeParse({
        ...binding,
        imageQueries: { imageGeneration: { url: unicodeUrl, auth: 'none' } },
      }).success
    ).toBe(true)
    for (const url of [
      'ftp://vendor.invalid/tasks/{taskId}',
      'https://fixture:secret@vendor.invalid/tasks/{taskId}',
      'https://vendor.invalid/tasks/{taskId}?key=fixture',
      'https://vendor.invalid/tasks/{taskId}#fragment',
      'https://vendor.invalid/tasks/../{taskId}',
      'https://vendor.invalid/tasks/%2e%2e/{taskId}',
      'https://vendor.invalid/tasks/%0a/{taskId}',
      'https://vendor.invalid/tasks/static',
      `https://vendor.invalid/tasks/${'图'.repeat(2800)}/{taskId}`,
      `https://vendor.invalid/tasks/${'x'.repeat(9216)}/{taskId}`,
    ]) {
      expect(
        providerBindingSchema.safeParse({
          ...binding,
          imageQueries: { imageGeneration: { url, auth: 'none' } },
        }).success
      ).toBe(false)
    }
    for (const invalid of [
      { ...binding, protocols: ['imageGeneration'] },
      {
        ...binding,
        imageQueries: { chat: binding.imageQueries.imageGeneration },
      },
      {
        ...binding,
        imageQueries: {
          imageGeneration: {
            ...binding.imageQueries.imageGeneration,
            auth: 'cookie',
          },
        },
      },
      {
        ...binding,
        imageQueries: {
          imageGeneration: {
            ...binding.imageQueries.imageGeneration,
            headers: {},
          },
        },
      },
    ])
      expect(providerBindingSchema.safeParse(invalid).success).toBe(false)
  })
  it('preserves host authentication and rejects unsafe or incompatible declarations', async () => {
    const manifest = {
      apiVersion: 2,
      kind: 'provider',
      key: 'auth',
      name: 'Host authentication',
      version: '1.0.0',
      protocols: ['chat'],
      credentialKinds: ['api_key'],
      hooks: ['request', 'response'],
    }
    const jwt = {
      keySeparator: '|',
      issuerClaim: 'iss',
      ttlSeconds: 1800,
      timeUnit: 'seconds',
      notBeforeSkewSeconds: 5,
    }
    const authentication = {
      type: 'jwt_hs256',
      header: 'Authorization',
      prefix: 'Bearer ',
      jwt,
    }
    const item = (value: unknown) => ({
      items: [{ key: 'auth', enabled: true, revision: 1, manifest: value }],
    })
    for (const valid of [
      authentication,
      { type: 'api_key', header: 'X-Vendor-Key', prefix: 'Key ' },
      {
        ...authentication,
        jwt: {
          ...jwt,
          keySeparator: '.',
          issuerClaim: 'api_key',
          timeUnit: 'milliseconds',
          issuedAtClaim: 'timestamp',
          signType: 'SIGN',
        },
      },
    ]) {
      const value = item({ ...manifest, authentication: valid })
      mock.mockResolvedValueOnce(value)
      await expect(channelPluginsApi.list()).resolves.toEqual(value)
    }
    for (const invalid of [
      ...[
        'Host',
        'Cookie',
        'Proxy-Authorization',
        'Sec-WebSocket-Key',
        'X-Forwarded-For',
        'Content-Length',
        'Session-Id',
        'Conversation_Id',
        'X-Codex-Turn-Metadata',
        'X-Claude-Code-Session-Id',
        'Anthropic-Version',
        'OpenAI-Beta',
        'OpenAI-Project',
        'X-Request-Id',
        'Traceparent',
        'X-Key\n',
      ].map((header) => ({ ...authentication, header })),
      { ...authentication, prefix: 'Bearer\r\n' },
      { ...authentication, prefix: '\u0085' },
      { ...authentication, jwt: undefined },
      { ...authentication, type: 'api_key' },
      { ...authentication, type: 'oauth2_jwt' },
      { ...authentication, jwt: { ...jwt, ttlSeconds: 59 } },
      { ...authentication, jwt: { ...jwt, ttlSeconds: 86401 } },
      { ...authentication, jwt: { ...jwt, ttlSeconds: 90.5 } },
      { ...authentication, jwt: { ...jwt, issuerClaim: 'exp' } },
      { ...authentication, jwt: { ...jwt, issuedAtClaim: 'iss' } },
      { ...authentication, jwt: { ...jwt, algorithm: 'RS256' } },
      null,
    ]) {
      mock.mockResolvedValueOnce(item({ ...manifest, authentication: invalid }))
      await expect(channelPluginsApi.list()).rejects.toThrow()
    }
    for (const credentialKinds of [['oauth'], ['none'], ['api_key', 'oauth']]) {
      mock.mockResolvedValueOnce(
        item({ ...manifest, credentialKinds, authentication })
      )
      await expect(channelPluginsApi.list()).rejects.toThrow()
    }
    mock.mockResolvedValueOnce(
      item({
        ...manifest,
        apiVersion: 1,
        kind: undefined,
        credentialKinds: undefined,
        authentication,
      })
    )
    await expect(channelPluginsApi.list()).rejects.toThrow()
  })
  it('accepts only the declared durable task capability and a persistent protocol', async () => {
    const manifest = {
      apiVersion: 2,
      kind: 'provider',
      key: 'capture',
      name: 'Capture',
      version: '1.0.0',
      protocols: ['video'],
      credentialKinds: ['api_key'],
      taskCapture: 'durable',
      hooks: ['submit', 'query', 'content', 'taskResult'],
    }
    const item = (value: unknown) => ({
      items: [{ key: 'capture', enabled: true, revision: 1, manifest: value }],
    })
    mock.mockResolvedValueOnce(item(manifest))
    await expect(channelPluginsApi.list()).resolves.toEqual(item(manifest))
    for (const invalid of [
      { ...manifest, taskCapture: 'legacy' },
      { ...manifest, taskCapture: true },
      { ...manifest, protocols: ['chat'], hooks: ['request', 'response'] },
    ]) {
      mock.mockResolvedValueOnce(item(invalid))
      await expect(channelPluginsApi.list()).rejects.toThrow()
    }
  })
  it('preserves explicit request forms and rejects invalid declarations', async () => {
    const manifest = {
      apiVersion: 2,
      kind: 'provider',
      key: 'upload',
      name: 'Upload',
      version: '1.0.0',
      protocols: ['video'],
      credentialKinds: ['api_key'],
      requestKinds: ['json', 'multipart'],
      hooks: ['submit', 'query', 'content', 'taskResult'],
    }
    const item = (value: unknown) => ({
      items: [{ key: 'upload', enabled: true, revision: 1, manifest: value }],
    })
    mock.mockResolvedValueOnce(item(manifest))
    await expect(channelPluginsApi.list()).resolves.toEqual(item(manifest))
    for (const requestKinds of [
      [],
      ['multipart', 'multipart'],
      ['form'],
      null,
    ]) {
      mock.mockResolvedValueOnce(item({ ...manifest, requestKinds }))
      await expect(channelPluginsApi.list()).rejects.toThrow()
    }
  })
  it('retains explicit anonymous provider metadata and catalog authentication', async () => {
    const manifest = {
      apiVersion: 2,
      kind: 'provider',
      key: 'anonymous',
      name: 'Anonymous',
      version: '1.0.0',
      protocols: ['messages'],
      credentialKinds: ['none'],
      hooks: ['request', 'response', 'modelsRequest', 'modelsResponse'],
      defaults: {
        endpoints: {
          messages: {
            path: '/v1/messages',
            auth: 'none',
            profile: 'ollama-messages',
          },
        },
        catalog: { path: '/inventory', auth: 'none' },
      },
    }
    const value = {
      items: [{ key: 'anonymous', enabled: true, revision: 1, manifest }],
    }
    mock.mockResolvedValueOnce(value)
    await expect(channelPluginsApi.list()).resolves.toEqual(value)
    const binding = {
      key: 'anonymous',
      protocols: ['messages'],
      catalog: { url: 'http://localhost:11434/inventory', auth: 'none' },
    }
    expect(providerBindingSchema.parse(binding)).toEqual(binding)
    mock.mockResolvedValueOnce({
      items: [
        {
          ...value.items[0],
          manifest: { ...manifest, credentialKinds: ['none', 'none'] },
        },
      ],
    })
    await expect(channelPluginsApi.list()).rejects.toThrow()
  })
  it('reads decoder hooks and original observation selectors without granting new protocols', async () => {
    const manifest = {
      apiVersion: 2,
      kind: 'provider',
      key: 'decoder',
      name: 'Decoder',
      version: '1.0.0',
      protocols: ['chat'],
      credentialKinds: ['api_key'],
      hooks: ['request', 'response', 'event', 'decodeResponse', 'decodeEvent'],
      observations: {
        chat: {
          response: {
            usagePaths: { prompt_tokens: '/a.b/a~1b/~0count' },
            modelPath: '/engine',
            taskIdPath: '/task/id',
          },
          event: {
            usagePaths: { completion_tokens: '/meter/output' },
            errorPath: '/failure',
          },
        },
      },
    }
    const item = (meta: unknown) => ({
      items: [{ key: 'decoder', enabled: true, revision: 1, manifest: meta }],
    })
    mock.mockResolvedValueOnce(item(manifest))
    await expect(channelPluginsApi.list()).resolves.toEqual(item(manifest))
    for (const invalid of [
      { ...manifest, observations: { responses: manifest.observations.chat } },
      {
        ...manifest,
        observations: {
          chat: { response: { usagePaths: { invoice: '/amount' } } },
        },
      },
      {
        ...manifest,
        observations: {
          chat: {
            response: { usagePaths: { prompt_tokens: '/bad~2pointer' } },
          },
        },
      },
      { ...manifest, observations: { chat: {} } },
      { ...manifest, observations: { chat: { response: { usagePaths: {} } } } },
    ]) {
      mock.mockResolvedValueOnce(item(invalid))
      await expect(channelPluginsApi.list()).rejects.toThrow()
    }
  })
  it('keeps provider manifests distinct and preserves their configured defaults', async () => {
    const value = {
      items: [
        {
          key: 'driver',
          revision: 1,
          enabled: true,
          manifest: {
            apiVersion: 2,
            kind: 'provider',
            key: 'driver',
            name: 'Driver',
            version: '1.0.0',
            protocols: ['chat'],
            credentialKinds: ['api_key'],
            hooks: ['request', 'response', 'modelsRequest', 'modelsResponse'],
            defaults: {
              endpoints: { chat: { path: '/generate', auth: 'bearer' } },
              catalog: {
                path: '/models',
                auth: 'bearer',
                cursorParameter: 'cursor',
              },
            },
          },
        },
      ],
    }
    mock.mockResolvedValueOnce(value)
    await expect(channelPluginsApi.list()).resolves.toEqual(value)
  })
  it('keeps compact and WebSocket as separate explicit binding slots', async () => {
    const protocols = [
      'chat',
      'responses',
      'messages',
      'gemini',
      'responsesCompact',
      'responsesWebsocket',
    ]
    const value = {
      items: [
        {
          key: 'native',
          revision: 1,
          enabled: true,
          manifest: {
            apiVersion: 1,
            key: 'native',
            name: 'Native',
            version: '1.0.0',
            protocols,
            hooks: ['request', 'event'],
          },
        },
      ],
    }
    mock.mockResolvedValueOnce(value)
    await expect(channelPluginsApi.list()).resolves.toEqual(value)
    expect(pluginBindingSchema.parse({ key: 'native', protocols })).toEqual({
      key: 'native',
      protocols,
    })
    for (const protocol of ['responsesCompact', 'responsesWebsocket']) {
      expect(
        pluginBindingSchema.parse({ key: 'native', protocols: [protocol] })
          .protocols
      ).toEqual([protocol])
    }
    expect(() =>
      pluginBindingSchema.parse({
        key: 'native',
        protocols: ['futureTransport'],
      })
    ).toThrow()
  })
  it('rejects malformed snapshots rather than accepting an empty list', async () => {
    for (const value of [
      {},
      { items: null },
      { items: [{ key: 'fixture', enabled: true, revision: 0 }] },
      { items: [{ key: 'fixture', enabled: true, revision: 1 }] },
      {
        items: [
          {
            key: 'fixture',
            enabled: true,
            revision: 1,
            manifest: {
              apiVersion: 1,
              key: 'other',
              name: 'Other',
              version: '1',
              protocols: ['chat'],
              hooks: ['request'],
            },
          },
        ],
      },
    ]) {
      mock.mockResolvedValueOnce(value)
      await expect(channelPluginsApi.list()).rejects.toThrow()
    }
  })
  it('keeps a damaged item available for management recovery', async () => {
    const value = {
      items: [
        {
          key: 'fixture',
          enabled: false,
          revision: 1,
          loadError: 'channel_plugin_load_failed',
        },
      ],
    }
    mock.mockResolvedValueOnce(value)
    await expect(channelPluginsApi.list()).resolves.toEqual(value)
  })
  it('rejects a wrong identity or revision on reads and writes', async () => {
    mock.mockResolvedValueOnce({
      key: 'other',
      revision: 4,
      enabled: true,
      source: 'synthetic',
    })
    await expect(channelPluginsApi.detail('fixture')).rejects.toThrow()
    for (const response of [
      {},
      { key: 'other', revision: 2, enabled: true },
      { key: 'fixture', revision: 1, enabled: true },
      { key: 'fixture', revision: 2, enabled: false },
    ]) {
      mock.mockResolvedValueOnce(response)
      await expect(
        channelPluginsApi.save('fixture', {
          source: 'synthetic',
          enabled: true,
          expectedRevision: 1,
        })
      ).rejects.toThrow()
    }
  })
})
