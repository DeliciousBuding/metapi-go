import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { MemoryRouter } from 'react-router-dom';
import { ToastProvider } from '../components/Toast.js';
import Accounts from './Accounts.js';
import { installAccountsSnapshotCompat } from './testApiCompat.js';

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    getAccounts: vi.fn(),
    getAccountsSnapshot: vi.fn(),
    getSites: vi.fn(),
    getAccountTokens: vi.fn(),
    addAccount: vi.fn(),
    addAccountAvailableModels: vi.fn(),
  },
}));

vi.mock('../api.js', () => ({
  api: apiMock,
}));

function collectText(root: ReactTestRenderer): string {
  return JSON.stringify(root.toJSON());
}

async function renderAccounts(accounts: unknown[]) {
  let root!: ReactTestRenderer;
  await act(async () => {
    root = create(
      <MemoryRouter initialEntries={['/accounts']}>
        <ToastProvider>
          <Accounts />
        </ToastProvider>
      </MemoryRouter>,
    );
  });
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
  return root;
}

const baseAccount = {
  id: 1,
  siteId: 1,
  username: 'truth-user',
  accessToken: 'sk-truth',
  status: 'active',
  balance: 10,
  balanceUsed: 2,
  site: { id: 1, name: 'Site A', status: 'active', platform: 'new-api' },
};

describe('Accounts per-account today metrics', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    installAccountsSnapshotCompat(apiMock);
    apiMock.getAccounts.mockResolvedValue([]);
    apiMock.getSites.mockResolvedValue([]);
    apiMock.getAccountTokens.mockResolvedValue([]);
    apiMock.addAccount.mockResolvedValue({ id: 99, siteId: 1, tokenType: 'apikey', queued: false });
    apiMock.addAccountAvailableModels.mockResolvedValue({ success: true });
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders real reward/spend when status is complete', async () => {
    apiMock.getAccounts.mockResolvedValue([
      {
        ...baseAccount,
        todayReward: 1.25,
        todayRewardStatus: 'complete',
        todaySpend: 0.2,
        todaySpendStatus: 'complete',
      },
    ]);
    const root = await renderAccounts([baseAccount]);
    const rendered = collectText(root);
    expect(rendered).toContain('+1.25');
    expect(rendered).toContain('-0.20');
  });

  it('shows em dash instead of fake zero when status is partial', async () => {
    apiMock.getAccounts.mockResolvedValue([
      {
        ...baseAccount,
        todayReward: 0,
        todayRewardStatus: 'partial',
        todayRewardReason: 'source_partial',
        todaySpend: 0,
        todaySpendStatus: 'partial',
      },
    ]);
    const root = await renderAccounts([baseAccount]);
    const rendered = collectText(root);
    expect(rendered).not.toContain('+0.00');
    expect(rendered).toContain('—');
  });

  it('shows em dash when backend omits metric fields (degraded path)', async () => {
    apiMock.getAccounts.mockResolvedValue([{ ...baseAccount }]);
    const root = await renderAccounts([baseAccount]);
    const rendered = collectText(root);
    expect(rendered).not.toContain('+0.00');
    expect(rendered).toContain('—');
  });
});
