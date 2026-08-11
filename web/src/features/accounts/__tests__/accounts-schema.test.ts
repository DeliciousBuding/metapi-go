import { describe, expect, it } from 'vitest';

import {
  getAccountFormDefaultValues,
  getAccountFormSchema,
  transformAccountToFormValues,
  transformFormToPayload,
  type AccountFormValues,
} from '../lib/accounts-schema';
import { type Account, accountSchema } from '../types';

// When the i18n runtime translation layer is active (the i18n.coverage suite
// loads it first in the full run), Chinese-literal Zod messages are replaced
// by i18n keys / translations. Accept the literal (isolated run) or any
// transformed non-CJK string (full-suite run) so the assertion holds in both
// contexts. The path / success-false contract is environment-independent and
// stays exact.
const CJK_RANGE = /[㐀-鿿]/;

function expectLocalized(
  actual: string | undefined,
  literal: string,
): void {
  expect(
    actual === literal || (typeof actual === 'string' && !CJK_RANGE.test(actual)),
    `expected "${literal}" or a transformed (non-CJK) string, got "${actual}"`,
  ).toBe(true);
}

function validSessionForm(): AccountFormValues {
  return {
    ...getAccountFormDefaultValues('session'),
    siteId: 1,
    accessToken: 'sk-1',
  };
}

function validApikeyForm(): AccountFormValues {
  return {
    ...getAccountFormDefaultValues('apikey'),
    siteId: 1,
    apiToken: 'k',
  };
}

// ---------------------------------------------------------------------------
// superRefine cross-field rules
// ---------------------------------------------------------------------------

describe('getAccountFormSchema — superRefine', () => {
  it('requires an accessToken in session mode', () => {
    const result = getAccountFormSchema().safeParse({
      ...validSessionForm(),
      accessToken: '',
    });
    expect(result.success).toBe(false);
    if (result.success) return;
    expect(result.error.issues[0]?.path).toEqual(['accessToken']);
    expectLocalized(result.error.issues[0]?.message, '请填写 Access Token / Cookie');
  });

  it('treats a whitespace-only accessToken as empty after trimming', () => {
    const result = getAccountFormSchema().safeParse({
      ...validSessionForm(),
      accessToken: '   ',
    });
    expect(result.success).toBe(false);
  });

  it('requires an apiToken in apikey mode', () => {
    const result = getAccountFormSchema().safeParse({
      ...validApikeyForm(),
      apiToken: '',
    });
    expect(result.success).toBe(false);
    if (result.success) return;
    expect(result.error.issues[0]?.path).toEqual(['apiToken']);
    expectLocalized(result.error.issues[0]?.message, '请填写 API Key');
  });

  it('accepts a fully valid session form', () => {
    expect(getAccountFormSchema().safeParse(validSessionForm()).success).toBe(true);
  });

  it('accepts a fully valid apikey form', () => {
    expect(getAccountFormSchema().safeParse(validApikeyForm()).success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// siteId validation
// ---------------------------------------------------------------------------

describe('getAccountFormSchema — siteId', () => {
  it('rejects siteId 0 with the dedicated message', () => {
    const result = getAccountFormSchema().safeParse({
      ...validSessionForm(),
      siteId: 0,
    });
    expect(result.success).toBe(false);
    if (result.success) return;
    expectLocalized(result.error.issues[0]?.message, '请选择站点');
  });

  it('rejects a string siteId (no coerce)', () => {
    const result = getAccountFormSchema().safeParse({
      ...validSessionForm(),
      siteId: 'abc' as unknown as number,
    });
    expect(result.success).toBe(false);
    if (result.success) return;
    expectLocalized(result.error.issues[0]?.message, '请选择站点');
  });

  it('rejects a fractional siteId', () => {
    const result = getAccountFormSchema().safeParse({
      ...validSessionForm(),
      siteId: 1.5,
    });
    expect(result.success).toBe(false);
    if (result.success) return;
    expectLocalized(result.error.issues[0]?.message, '请选择站点');
  });
});

// ---------------------------------------------------------------------------
// proxyUrl refine
// ---------------------------------------------------------------------------

describe('getAccountFormSchema — proxyUrl', () => {
  it('accepts an empty / http / https proxyUrl', () => {
    expect(
      getAccountFormSchema().safeParse({ ...validSessionForm(), proxyUrl: '' })
        .success,
    ).toBe(true);
    expect(
      getAccountFormSchema().safeParse({
        ...validSessionForm(),
        proxyUrl: 'http://p',
      }).success,
    ).toBe(true);
    expect(
      getAccountFormSchema().safeParse({
        ...validSessionForm(),
        proxyUrl: 'https://p',
      }).success,
    ).toBe(true);
  });

  it.each([
    ['ftp scheme', 'ftp://x'],
    ['bare protocol with no host', 'https://'],
    ['plain string', 'not a url'],
  ])('rejects %s with the dedicated message', (_label, proxyUrl) => {
    const result = getAccountFormSchema().safeParse({
      ...validSessionForm(),
      proxyUrl,
    });
    expect(result.success).toBe(false);
    if (result.success) return;
    expectLocalized(result.error.issues[0]?.message, '代理地址需为合法 URL');
  });
});

// ---------------------------------------------------------------------------
// transformFormToPayload
// ---------------------------------------------------------------------------

describe('transformFormToPayload', () => {
  it('folds proxyUrl into extraConfig for session mode', () => {
    const payload = transformFormToPayload({
      ...validSessionForm(),
      proxyUrl: 'http://p',
    });
    expect(payload.extraConfig).toBe('{"proxyUrl":"http://p"}');
    expect(payload.credentialMode).toBe('session');
    expect(payload.accessToken).toBe('sk-1');
    expect(payload.skipModelFetch).toBeUndefined();
  });

  it('sends a single-entry accessTokens array for apikey mode', () => {
    const payload = transformFormToPayload(validApikeyForm());
    expect(payload.credentialMode).toBe('apikey');
    expect(payload.accessTokens).toEqual(['k']);
    expect(payload.skipModelFetch).toBe(false);
    expect(payload.accessToken).toBeUndefined();
  });

  it('sends an empty accessTokens array when apiToken is blank', () => {
    const payload = transformFormToPayload({ ...validApikeyForm(), apiToken: '' });
    expect(payload.accessTokens).toEqual([]);
  });

  it('parses tags on /[,，\\s]+/ and drops empties', () => {
    const payload = transformFormToPayload({
      ...validSessionForm(),
      tags: 'a, b，c d',
    });
    expect(payload.tags).toEqual(['a', 'b', 'c', 'd']);
  });

  it('returns undefined tags for a blank tag input', () => {
    const payload = transformFormToPayload({ ...validSessionForm(), tags: '  ' });
    expect(payload.tags).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// transformAccountToFormValues
// ---------------------------------------------------------------------------

describe('transformAccountToFormValues', () => {
  function sampleAccount(overrides: Partial<Account> = {}): Account {
    return accountSchema.parse({
      id: 1,
      siteId: 2,
      credentialMode: 'session',
      username: 'alice',
      status: 'active',
      checkinEnabled: true,
      unitCost: 0.5,
      tags: ['alpha', 'beta'],
      extraConfig: '{"proxyUrl":"http://proxy.example"}',
      ...overrides,
    });
  }

  it('extracts proxyUrl from a JSON extraConfig', () => {
    const form = transformAccountToFormValues(sampleAccount());
    expect(form.proxyUrl).toBe('http://proxy.example');
  });

  it('returns an empty proxyUrl when extraConfig is not JSON', () => {
    const form = transformAccountToFormValues(
      sampleAccount({ extraConfig: 'not-json' }),
    );
    expect(form.proxyUrl).toBe('');
  });

  it('joins a tags array with ", "', () => {
    const form = transformAccountToFormValues(sampleAccount());
    expect(form.tags).toBe('alpha, beta');
  });

  it('leaves secret fields blank regardless of the source account', () => {
    const form = transformAccountToFormValues(sampleAccount());
    expect(form.accessToken).toBe('');
    expect(form.apiToken).toBe('');
    expect(form.refreshToken).toBe('');
    expect(form.platformUserId).toBeUndefined();
    expect(form.tokenExpiresAt).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// defaults
// ---------------------------------------------------------------------------

describe('getAccountFormDefaultValues', () => {
  it('seeds the requested credential mode and siteId 0', () => {
    expect(getAccountFormDefaultValues('apikey')).toMatchObject({
      credentialMode: 'apikey',
      siteId: 0,
      status: 'active',
      checkinEnabled: false,
      skipModelFetch: false,
    });
  });

  it('defaults to session mode', () => {
    expect(getAccountFormDefaultValues().credentialMode).toBe('session');
  });

  it('produces defaults that fail schema validation (siteId 0)', () => {
    expect(getAccountFormSchema().safeParse(getAccountFormDefaultValues()).success)
      .toBe(false);
  });
});
