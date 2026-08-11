import { describe, expect, it } from 'vitest';

import {
  PROXY_LOG_STATUS_FILTER_OPTIONS,
  SORTING_ITEM_SCHEMA,
  proxyLogsSearchSchema,
} from '../lib/proxy-logs-schema';

// ---------------------------------------------------------------------------
// sort transform
// ---------------------------------------------------------------------------

describe('proxyLogsSearchSchema — sort transform', () => {
  it('splits multi-segment sort strings into ordered items', () => {
    const result = proxyLogsSearchSchema.parse({ sort: 'created:desc,model:asc' });
    expect(result.sort).toEqual([
      { id: 'created', desc: true },
      { id: 'model', desc: false },
    ]);
  });

  it('defaults direction to asc when only the id is present', () => {
    const result = proxyLogsSearchSchema.parse({ sort: 'model' });
    expect(result.sort).toEqual([{ id: 'model', desc: false }]);
  });

  it('returns an empty array when sort is missing or empty', () => {
    expect(proxyLogsSearchSchema.parse({}).sort).toEqual([]);
    expect(proxyLogsSearchSchema.parse({ sort: '' }).sort).toEqual([]);
    expect(proxyLogsSearchSchema.parse({ sort: undefined }).sort).toEqual([]);
  });

  it('keeps a bare direction token as an empty id', () => {
    const result = proxyLogsSearchSchema.parse({ sort: ':desc' });
    expect(result.sort).toEqual([{ id: '', desc: true }]);
  });
});

// ---------------------------------------------------------------------------
// status enum
// ---------------------------------------------------------------------------

describe('proxyLogsSearchSchema — status enum', () => {
  it('accepts each documented status', () => {
    expect(proxyLogsSearchSchema.parse({ status: 'all' }).status).toBe('all');
    expect(proxyLogsSearchSchema.parse({ status: 'success' }).status).toBe(
      'success',
    );
    expect(proxyLogsSearchSchema.parse({ status: 'failed' }).status).toBe(
      'failed',
    );
  });

  it('rejects an unknown status with an enum error', () => {
    const result = proxyLogsSearchSchema.safeParse({ status: 'partial' });
    expect(result.success).toBe(false);
    if (result.success) return;
    expect(result.error.issues[0]?.message).toContain('Invalid option');
  });
});

// ---------------------------------------------------------------------------
// numeric coercion + bounds
// ---------------------------------------------------------------------------

describe('proxyLogsSearchSchema — numerics', () => {
  it('coerces string numerics', () => {
    const result = proxyLogsSearchSchema.parse({
      page: '5',
      pageSize: '50',
      siteId: '7',
      latencyMin: '10',
      latencyMax: '100',
    });
    expect(result.page).toBe(5);
    expect(result.pageSize).toBe(50);
    expect(result.siteId).toBe(7);
    expect(result.latencyMin).toBe(10);
    expect(result.latencyMax).toBe(100);
  });

  it('accepts page 0 (min 0, unlike the checkin schema)', () => {
    expect(proxyLogsSearchSchema.parse({ page: '0' }).page).toBe(0);
  });

  it.each([
    ['pageSize below 1', { pageSize: '0' }],
    ['pageSize above 200', { pageSize: '201' }],
    ['latencyMin negative', { latencyMin: '-1' }],
    ['non-numeric page', { page: 'abc' }],
  ])('rejects %s', (_label, input) => {
    const result = proxyLogsSearchSchema.safeParse(input);
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// SORTING_ITEM_SCHEMA
// ---------------------------------------------------------------------------

describe('SORTING_ITEM_SCHEMA', () => {
  it('parses a well-formed item', () => {
    expect(SORTING_ITEM_SCHEMA.parse({ id: 'created', desc: true })).toEqual({
      id: 'created',
      desc: true,
    });
  });

  it('requires both id and desc', () => {
    expect(
      SORTING_ITEM_SCHEMA.safeParse({ id: 'created' }).success,
    ).toBe(false);
    expect(
      SORTING_ITEM_SCHEMA.safeParse({ desc: true }).success,
    ).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// PROXY_LOG_STATUS_FILTER_OPTIONS
// ---------------------------------------------------------------------------

describe('PROXY_LOG_STATUS_FILTER_OPTIONS', () => {
  it('exposes all / success / failed in that order', () => {
    expect(PROXY_LOG_STATUS_FILTER_OPTIONS).toHaveLength(3);
    expect(PROXY_LOG_STATUS_FILTER_OPTIONS.map((o) => o.value)).toEqual([
      'all',
      'success',
      'failed',
    ]);
  });
});
