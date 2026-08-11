import { describe, expect, it } from 'vitest';

import {
  OAUTH_COLUMN_FILTER_ITEM_SCHEMA,
  OAUTH_PAGINATION_SCHEMA,
  OAUTH_SORTING_ITEM_SCHEMA,
  OAUTH_START_DEFAULT_VALUES,
  oauthSearchSchema,
  oauthStartSchema,
  type OAuthStartValues,
} from '../lib/oauth-schema';

function validOAuthStart(): OAuthStartValues {
  return { ...OAUTH_START_DEFAULT_VALUES, provider: 'github' };
}

// ---------------------------------------------------------------------------
// happy path
// ---------------------------------------------------------------------------

describe('oauthStartSchema — happy path', () => {
  it('parses a minimal valid form (provider only)', () => {
    expect(oauthStartSchema.safeParse(validOAuthStart()).success).toBe(true);
  });

  it('allows a blank projectId', () => {
    expect(
      oauthStartSchema.safeParse({ ...validOAuthStart(), projectId: '' }).success,
    ).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// provider required
// ---------------------------------------------------------------------------

describe('oauthStartSchema — provider', () => {
  it('rejects an empty provider with providerRequired', () => {
    const result = oauthStartSchema.safeParse({ ...validOAuthStart(), provider: '' });
    expect(result.success).toBe(false);
    if (result.success) return;
    expect(result.error.issues[0]?.message).toBe(
      'oauth.form.errors.providerRequired',
    );
  });

  it('rejects a whitespace-only provider after trimming', () => {
    expect(
      oauthStartSchema.safeParse({ ...validOAuthStart(), provider: '   ' }).success,
    ).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// proxyUrl refine
// ---------------------------------------------------------------------------

describe('oauthStartSchema — proxyUrl', () => {
  it('accepts an empty / http / https proxyUrl', () => {
    expect(
      oauthStartSchema.safeParse({ ...validOAuthStart(), proxyUrl: '' }).success,
    ).toBe(true);
    expect(
      oauthStartSchema.safeParse({
        ...validOAuthStart(),
        proxyUrl: 'https://proxy.example',
      }).success,
    ).toBe(true);
  });

  it.each([
    ['ftp scheme', 'ftp://x'],
    ['plain string', 'not-a-url'],
  ])('rejects %s with invalidProxyUrl', (_label, proxyUrl) => {
    const result = oauthStartSchema.safeParse({ ...validOAuthStart(), proxyUrl });
    expect(result.success).toBe(false);
    if (result.success) return;
    expect(result.error.issues[0]?.message).toBe(
      'oauth.form.errors.invalidProxyUrl',
    );
  });
});

// ---------------------------------------------------------------------------
// useSystemProxy boolean (no coerce)
// ---------------------------------------------------------------------------

describe('oauthStartSchema — useSystemProxy', () => {
  it('rejects a string flag (no coerce)', () => {
    const result = oauthStartSchema.safeParse({
      ...validOAuthStart(),
      useSystemProxy: 'true' as unknown as boolean,
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// defaults
// ---------------------------------------------------------------------------

describe('OAUTH_START_DEFAULT_VALUES', () => {
  it('exposes the canonical default shape', () => {
    expect(OAUTH_START_DEFAULT_VALUES).toEqual({
      provider: '',
      projectId: '',
      proxyUrl: '',
      useSystemProxy: false,
    });
  });

  it('fails schema validation (provider empty)', () => {
    expect(oauthStartSchema.safeParse(OAUTH_START_DEFAULT_VALUES).success).toBe(
      false,
    );
  });
});

// ---------------------------------------------------------------------------
// search schema
// ---------------------------------------------------------------------------

describe('oauthSearchSchema', () => {
  it('accepts page 0 and any status string (no enum)', () => {
    expect(
      oauthSearchSchema.parse({ page: '0', status: 'whatever' }),
    ).toMatchObject({ page: 0, status: 'whatever' });
  });

  it('parses multi-segment sort', () => {
    expect(oauthSearchSchema.parse({ sort: 'created:desc,model:asc' }).sort).toEqual(
      [
        { id: 'created', desc: true },
        { id: 'model', desc: false },
      ],
    );
  });

  it('rejects pageSize above 200', () => {
    expect(oauthSearchSchema.safeParse({ pageSize: '201' }).success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// shared item schemas
// ---------------------------------------------------------------------------

describe('shared OAuth item schemas', () => {
  it('OAUTH_PAGINATION_SCHEMA applies defaults', () => {
    expect(OAUTH_PAGINATION_SCHEMA.parse({})).toEqual({
      pageIndex: 0,
      pageSize: 20,
    });
  });

  it('OAUTH_SORTING_ITEM_SCHEMA requires both fields', () => {
    expect(
      OAUTH_SORTING_ITEM_SCHEMA.safeParse({ id: 'a' }).success,
    ).toBe(false);
    expect(
      OAUTH_SORTING_ITEM_SCHEMA.safeParse({ id: 'a', desc: true }).success,
    ).toBe(true);
  });

  it('OAUTH_COLUMN_FILTER_ITEM_SCHEMA accepts string / string[] / boolean', () => {
    expect(
      OAUTH_COLUMN_FILTER_ITEM_SCHEMA.safeParse({ id: 'a', value: 'x' }).success,
    ).toBe(true);
    expect(
      OAUTH_COLUMN_FILTER_ITEM_SCHEMA.safeParse({ id: 'a', value: 42 }).success,
    ).toBe(false);
  });
});
