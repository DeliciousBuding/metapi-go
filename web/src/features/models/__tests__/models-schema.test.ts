import { describe, expect, it } from 'vitest';

import {
  PAGINATION_SCHEMA,
  SORTING_ITEM_SCHEMA,
  modelsSearchSchema,
} from '../lib/models-schema';

// ---------------------------------------------------------------------------
// brand / capability transforms
// ---------------------------------------------------------------------------

describe('modelsSearchSchema — brand transform', () => {
  it('normalizes a comma-separated list (trims + drops empties) to a string', () => {
    expect(modelsSearchSchema.parse({ brand: 'openai, anthropic' }).brand).toBe(
      'openai,anthropic',
    );
  });

  it('drops empty segments and surrounding whitespace', () => {
    expect(modelsSearchSchema.parse({ brand: ' openai ,,' }).brand).toBe(
      'openai',
    );
  });

  it('returns undefined for missing / empty input (no URL noise)', () => {
    expect(modelsSearchSchema.parse({}).brand).toBeUndefined();
    expect(modelsSearchSchema.parse({ brand: '' }).brand).toBeUndefined();
    expect(modelsSearchSchema.parse({ brand: '[]' }).brand).toBeUndefined();
  });
});

describe('modelsSearchSchema — capability transform', () => {
  it('is independent of brand', () => {
    const result = modelsSearchSchema.parse({
      brand: 'openai',
      capability: 'vision, ,code',
    });
    expect(result.brand).toBe('openai');
    expect(result.capability).toBe('vision,code');
  });
});

// ---------------------------------------------------------------------------
// sort transform (shares the proxy-logs shape)
// ---------------------------------------------------------------------------

describe('modelsSearchSchema — sort transform', () => {
  it('normalizes a comma-separated sort descriptor to a canonical string', () => {
    expect(
      modelsSearchSchema.parse({ sort: 'created:desc,model:asc' }).sort,
    ).toBe('created:desc,model:asc');
  });

  it('returns undefined when sort is missing / empty (no URL noise)', () => {
    expect(modelsSearchSchema.parse({}).sort).toBeUndefined();
    expect(modelsSearchSchema.parse({ sort: '[]' }).sort).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// numerics
// ---------------------------------------------------------------------------

describe('modelsSearchSchema — numerics', () => {
  it('coerces string numerics and accepts page 0', () => {
    const result = modelsSearchSchema.parse({ page: '0', pageSize: '50' });
    expect(result.page).toBe(0);
    expect(result.pageSize).toBe(50);
  });

  it.each([
    ['pageSize below 1', { pageSize: '0' }],
    ['pageSize above 200', { pageSize: '201' }],
    ['non-numeric page', { page: 'abc' }],
  ])('rejects %s', (_label, input) => {
    expect(modelsSearchSchema.safeParse(input).success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// PAGINATION_SCHEMA + SORTING_ITEM_SCHEMA
// ---------------------------------------------------------------------------

describe('PAGINATION_SCHEMA', () => {
  it('applies defaults to an empty input', () => {
    expect(PAGINATION_SCHEMA.parse({})).toEqual({
      pageIndex: 0,
      pageSize: 20,
    });
  });

  it('coerces string numerics', () => {
    expect(
      PAGINATION_SCHEMA.parse({ pageIndex: '3', pageSize: '50' }),
    ).toEqual({ pageIndex: 3, pageSize: 50 });
  });

  it('rejects pageSize above 200', () => {
    expect(PAGINATION_SCHEMA.safeParse({ pageSize: '300' }).success).toBe(false);
  });
});

describe('SORTING_ITEM_SCHEMA', () => {
  it('requires both id and desc', () => {
    expect(SORTING_ITEM_SCHEMA.safeParse({ id: 'a' }).success).toBe(false);
    expect(
      SORTING_ITEM_SCHEMA.safeParse({ id: 'a', desc: true }).success,
    ).toBe(true);
  });
});
