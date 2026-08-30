// metapi-go/lib — section-registry tests (S8 三合一).
//
// Pins the shared factory that replaced the three per-feature clones
// (settings / dashboard / observability): id order, nav URL styles (path vs
// query), readonly badge passthrough, and the sections[0] fallback for
// unknown ids.

import { describe, expect, it } from 'vitest'

import {
  createSectionRegistry,
  type SectionRegistry,
} from '../section-registry'

type TestSectionId = 'general' | 'appearance'

function buildRegistry(
  overrides: Partial<
    Parameters<typeof createSectionRegistry<TestSectionId>>[0]
  > = {}
): SectionRegistry<TestSectionId> {
  return createSectionRegistry<TestSectionId>({
    sections: [
      {
        id: 'general',
        title: 'General',
        description: 'General settings',
        build: () => 'general-content',
      },
      {
        id: 'appearance',
        title: 'Appearance',
        readonly: true,
        build: () => 'appearance-content',
      },
    ],
    defaultSection: 'general',
    basePath: '/settings/general',
    ...overrides,
  })
}

describe('createSectionRegistry — ids + default', () => {
  it('exposes section ids in declaration order', () => {
    expect(buildRegistry().sectionIds).toEqual(['general', 'appearance'])
  })

  it('passes the defaultSection through unchanged', () => {
    expect(buildRegistry().defaultSection).toBe('general')
  })
})

describe('createSectionRegistry — getSectionNavItems', () => {
  it('builds path-style urls by default (basePath/id)', () => {
    expect(buildRegistry().getSectionNavItems()).toEqual([
      { title: 'General', url: '/settings/general/general' },
      {
        title: 'Appearance',
        url: '/settings/general/appearance',
        readonly: true,
      },
    ])
  })

  it('builds query-style urls when urlStyle is query', () => {
    expect(buildRegistry({ urlStyle: 'query' }).getSectionNavItems()).toEqual([
      { title: 'General', url: '/settings/general?section=general' },
      {
        title: 'Appearance',
        url: '/settings/general?section=appearance',
        readonly: true,
      },
    ])
  })
})

describe('createSectionRegistry — getSectionMeta', () => {
  it('returns the matching section for a known id', () => {
    const meta = buildRegistry().getSectionMeta('appearance')
    expect(meta.id).toBe('appearance')
    expect(meta.title).toBe('Appearance')
  })

  it('falls back to sections[0] for an unknown id', () => {
    const meta = buildRegistry().getSectionMeta('nonexistent' as TestSectionId)
    expect(meta.id).toBe('general')
  })
})

describe('createSectionRegistry — getSectionContent', () => {
  it('renders the matched section via its build()', () => {
    expect(buildRegistry().getSectionContent('appearance')).toBe(
      'appearance-content'
    )
  })

  it('renders sections[0] content for an unknown id', () => {
    expect(
      buildRegistry().getSectionContent('nonexistent' as TestSectionId)
    ).toBe('general-content')
  })
})
