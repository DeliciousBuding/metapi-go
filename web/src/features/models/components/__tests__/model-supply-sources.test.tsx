import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import type { ModelSupplySource } from '../../types'
import { ModelSupplySources } from '../model-supply-sources'

afterEach(cleanup)

const source: ModelSupplySource = {
  kind: 'direct',
  id: 3,
  name: 'Primary Key',
  connectionName: 'Coding connection',
  upstreamModel: 'qwen-coder',
  evidence: 'configured',
  enabled: false,
  credentialKind: 'api_key',
  successCount: 0,
  failCount: 0,
  bindings: [
    {
      itemId: 4,
      routeId: 2,
      publicModel: 'coding',
      routeEnabled: true,
      groupEnabled: false,
      selected: false,
    },
  ],
}

describe('model supply sources', () => {
  it('explains public bindings, disabled configuration and zero observed requests without claiming availability', () => {
    render(<ModelSupplySources sources={[source]} />)
    expect(screen.getByText('Coding connection')).toBeInTheDocument()
    expect(screen.getByText('coding → qwen-coder')).toBeInTheDocument()
    expect(screen.getByText('Configuration disabled')).toBeInTheDocument()
    expect(screen.getByText('Group disabled')).toBeInTheDocument()
    expect(screen.getByText('Not selected in manual group')).toBeInTheDocument()
    expect(
      screen.getByText('Historical requests: 0 succeeded · 0 failed')
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'Configured supply; not an inference verification result.'
      )
    ).toBeInTheDocument()
  })

  it('distinguishes OAuth availability records from direct credential configuration', () => {
    render(
      <ModelSupplySources
        sources={[
          {
            ...source,
            kind: 'oauth',
            evidence: 'availability',
            enabled: true,
            bindings: [],
            successCount: undefined,
            failCount: undefined,
          },
        ]}
      />
    )
    expect(screen.getByText('OAuth account')).toBeInTheDocument()
    expect(screen.getByText('Recorded model')).toBeInTheDocument()
    expect(screen.queryByText('Upstream model')).not.toBeInTheDocument()
    expect(
      screen.getByText(
        'Availability record; may come from discovery or manual configuration.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(/Historical requests:/)).not.toBeInTheDocument()
  })

  it('keeps absent server data distinct from a known empty supply list', () => {
    const view = render(<ModelSupplySources sources={undefined} />)
    expect(
      screen.getByText('Supply details are not available from this server.')
    ).toBeInTheDocument()
    view.rerender(<ModelSupplySources sources={[]} />)
    expect(
      screen.getByText(
        'No supply record for this model. Check its route configuration.'
      )
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Supply details are not available from this server.')
    ).not.toBeInTheDocument()
  })
})
