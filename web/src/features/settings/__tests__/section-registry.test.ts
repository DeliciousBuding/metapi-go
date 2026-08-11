import { describe, expect, it } from 'vitest';

import { createSectionRegistry } from '../utils/section-registry';
import type { SectionRegistry } from '../utils/section-registry';

type TestSectionId = 'general' | 'appearance';

function buildRegistry(): SectionRegistry<TestSectionId> {
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
        build: () => 'appearance-content',
      },
    ],
    defaultSection: 'general',
    basePath: '/settings/general',
  });
}

// ---------------------------------------------------------------------------
// sectionIds + defaultSection
// ---------------------------------------------------------------------------

describe('createSectionRegistry — ids + default', () => {
  it('exposes section ids in declaration order', () => {
    expect(buildRegistry().sectionIds).toEqual(['general', 'appearance']);
  });

  it('passes the defaultSection through unchanged', () => {
    expect(buildRegistry().defaultSection).toBe('general');
  });
});

// ---------------------------------------------------------------------------
// getSectionNavItems
// ---------------------------------------------------------------------------

describe('createSectionRegistry — getSectionNavItems', () => {
  it('builds one nav item per section with url = basePath/id', () => {
    expect(buildRegistry().getSectionNavItems()).toEqual([
      { title: 'General', url: '/settings/general/general' },
      { title: 'Appearance', url: '/settings/general/appearance' },
    ]);
  });
});

// ---------------------------------------------------------------------------
// getSectionMeta
// ---------------------------------------------------------------------------

describe('createSectionRegistry — getSectionMeta', () => {
  it('returns the matching section for a known id', () => {
    const meta = buildRegistry().getSectionMeta('appearance');
    expect(meta.id).toBe('appearance');
    expect(meta.title).toBe('Appearance');
  });

  it('falls back to sections[0] for an unknown id', () => {
    const meta = buildRegistry().getSectionMeta('nonexistent' as TestSectionId);
    expect(meta.id).toBe('general');
  });
});

// ---------------------------------------------------------------------------
// getSectionContent
// ---------------------------------------------------------------------------

describe('createSectionRegistry — getSectionContent', () => {
  it('renders the matched section via its build()', () => {
    expect(buildRegistry().getSectionContent('appearance')).toBe(
      'appearance-content',
    );
  });

  it('renders sections[0] content for an unknown id', () => {
    expect(buildRegistry().getSectionContent('nonexistent' as TestSectionId)).toBe(
      'general-content',
    );
  });
});
