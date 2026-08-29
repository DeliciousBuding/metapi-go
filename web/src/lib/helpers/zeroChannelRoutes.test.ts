import { describe, expect, it } from 'vitest'

import type { MissingTokenModelsByName } from './routeMissingTokenHints.js'
import type { RouteSummaryRow } from './token-route-contract.js'
import { buildZeroChannelPlaceholderRoutes } from './zeroChannelRoutes.js'

describe('buildZeroChannelPlaceholderRoutes', () => {
  it('merges missing-token and missing-group models into exact zero-channel placeholders', () => {
    const routes: RouteSummaryRow[] = [
      {
        id: 1,
        modelPattern: 'gpt-5-mini',
        displayName: 'gpt-5-mini',
        displayIcon: null,
        modelMapping: null,
        routingStrategy: 'weighted',
        enabled: true,
        channelCount: 1,
        enabledChannelCount: 1,
        siteNames: ['site-a'],
        decisionSnapshot: null,
        decisionRefreshedAt: null,
      },
    ]

    const modelsWithoutToken: MissingTokenModelsByName = {
      'gpt-5-mini': [
        { accountId: 1, username: 'alice', siteId: 11, siteName: 'site-a' },
      ],
      'gpt-5.2-codex': [
        { accountId: 2, username: 'bob', siteId: 12, siteName: 'site-b' },
      ],
    }
    const modelsMissingTokenGroups: MissingTokenModelsByName = {
      'gpt-5.2-codex': [
        {
          accountId: 3,
          username: 'cici',
          siteId: 13,
          siteName: 'site-c',
          missingGroups: ['opus'],
          requiredGroups: ['default', 'opus'],
          availableGroups: ['default'],
        },
      ],
      're:^claude-.*$': [
        { accountId: 4, username: 'dan', siteId: 14, siteName: 'site-d' },
      ],
    }

    const placeholders = buildZeroChannelPlaceholderRoutes(
      routes,
      modelsWithoutToken,
      modelsMissingTokenGroups
    )

    expect(placeholders).toHaveLength(1)
    expect(placeholders[0]).toMatchObject({
      modelPattern: 'gpt-5.2-codex',
      channelCount: 0,
      enabledChannelCount: 0,
      enabled: false,
      decisionSnapshot: null,
      kind: 'zero_channel',
      readOnly: true,
      isVirtual: true,
    })
    expect(placeholders[0].siteNames).toEqual(['site-b', 'site-c'])
    expect(placeholders[0].id).toBeLessThan(0)
  })
})
