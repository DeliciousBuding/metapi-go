import { afterAll, vi } from 'vitest';
import type { ReactElement, ReactNode } from 'react';
import type {
  ReactTestRenderer,
  TestRendererOptions,
} from 'react-test-renderer';

// jsdom does not implement window.scrollTo/scrollBy — every call prints
// "Not implemented: Window's scrollTo() method" to stderr. Stub them so the
// full-suite output stays clean and failures are easy to spot.
if (typeof window !== 'undefined') {
  Object.defineProperty(window, 'scrollTo', { value: () => {}, writable: true });
  Object.defineProperty(window, 'scrollBy', { value: () => {}, writable: true });
}

// Node 25+ ships an experimental global `localStorage` (`--localstorage-file`).
// When the file path is invalid the global exists but has no working getItem —
// that breaks components reading `localStorage` directly (e.g. RealtimeOpsPanel
// → getAuthToken). jsdom's window.localStorage is always complete; prefer it.
if (
  typeof globalThis !== 'undefined'
  && typeof window !== 'undefined'
  && typeof (globalThis as { localStorage?: Storage }).localStorage !== 'undefined'
  && typeof (globalThis as { localStorage?: Storage }).localStorage?.getItem !== 'function'
) {
  try {
    Object.defineProperty(globalThis, 'localStorage', {
      value: window.localStorage,
      configurable: true,
    });
  } catch {
    /* non-configurable global in some environments — jsdom path still fine */
  }
}

// React 19 concurrent mode needs an act-enabled environment; without it RTR
// trees unmount before tests can read `.root`.
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// react-test-renderer cannot host ReactDOM portals. Under jsdom, production
// portal helpers would otherwise mix renderers and crash with
// `parentInstance.children.indexOf is not a function`. Keep portal children
// inline in the RTR tree so existing component assertions keep working.
vi.mock('react-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-dom')>();
  return {
    ...actual,
    createPortal: ((children: ReactNode) => children) as typeof actual.createPortal,
  };
});

// Auto-wrap create() in act so effects/state flush before assertions. Existing
// suites call create() without act; React 19 otherwise unmounts immediately.
// ESM exports are not assignable, so mock the module instead of mutating it.
vi.mock('react-test-renderer', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-test-renderer')>();
  const { act, create: originalCreate } = actual;

  function createWithAct(
    element: ReactElement,
    options?: TestRendererOptions,
  ): ReactTestRenderer {
    let renderer!: ReactTestRenderer;
    act(() => {
      renderer = originalCreate(element, options);
    });
    return renderer;
  }

  const actualRecord = actual as typeof actual & {
    default?: { create?: typeof actual.create };
  };
  const patched = {
    ...actual,
    create: createWithAct,
  };

  if (actualRecord.default && typeof actualRecord.default === 'object') {
    return {
      ...patched,
      default: {
        ...actualRecord.default,
        create: createWithAct,
      },
    };
  }

  return patched;
});


// Dashboard charts pull @visactor/* ESM that breaks under vitest/jsdom.
// Provide lightweight stubs so page tests can render without native chart deps.
vi.mock('@visactor/react-vchart', () => ({
  VChart: () => null,
  default: () => null,
}))
vi.mock('@visactor/vchart', () => ({
  default: class VChartStub {},
  VChart: class VChartStub {},
}))

// Quiet noisy chart/React act warnings that otherwise queue console RPC and
// contribute to EnvironmentTeardownError under single-worker jsdom.
const originalError = console.error.bind(console);
const originalWarn = console.warn.bind(console);
console.error = (...args: unknown[]) => {
  const head = String(args[0] ?? '');
  if (head.includes('not wrapped in act') || head.includes('ReactDOMTestUtils')) return;
  originalError(...args);
};
console.warn = (...args: unknown[]) => {
  const head = String(args[0] ?? '');
  if (head.includes('react-test-renderer is deprecated')) return;
  originalWarn(...args);
};

// #266: Global afterAll to drain mocks and pending microtasks after each
// suite, reducing EnvironmentTeardownError from leftover console RPC.
afterAll(async () => {
  try {
    vi.clearAllMocks();
  } catch {
    // Ignore mock cleanup races under single-worker vitest.
  }
  // Drain pending microtasks so worker teardown does not race console RPC.
  await Promise.resolve();
  await Promise.resolve();
});
