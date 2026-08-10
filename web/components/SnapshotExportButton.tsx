import React, { useState } from 'react';
import { api } from '../api.js';
import { useToast } from './Toast.js';
import { tr } from '../i18n.js';

/* ------------------------------------------------------------------ */
/*  Canvas snapshot palette (fixed — CSS variables do not reach canvas) */
/* ------------------------------------------------------------------ */

const PALETTE = {
  primary: '#1a73e8',
  bg: '#f7f8fa',
  card: '#ffffff',
  text: '#111827',
  muted: '#6b7280',
  border: '#e5e7eb',
  success: '#16a34a',
  danger: '#dc2626',
  warn: '#f59e0b',
};

// Accent presets mapped to their light-theme primary, so the exported
// PNG matches the operator's chosen brand color. Falls back to blue.
const ACCENT_PRIMARY: Record<string, string> = {
  blue: '#1a73e8',
  indigo: '#3949ab',
  teal: '#00897b',
};

function currentAccentPrimary(): string {
  try {
    const accent =
      typeof document !== 'undefined'
        ? document.documentElement.getAttribute('data-accent')
        : null;
    return (accent && ACCENT_PRIMARY[accent]) || PALETTE.primary;
  } catch {
    return PALETTE.primary;
  }
}

const W = 1200;
const H = 630;

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

/**
 * SnapshotExportButton — shareable dashboard snapshot PNG. Draws a clean 1200×630 summary card on a native canvas (no
 * new dependency) from the dashboard snapshot + site distribution APIs and
 * downloads it as metapi-snapshot-YYYYMMDD.png. Dark text on light card —
 * readable in chat apps and issue comments.
 */
export default function SnapshotExportButton() {
  const toast = useToast();
  const [exporting, setExporting] = useState(false);

  const exportSnapshot = async () => {
    if (exporting) return;
    setExporting(true);
    try {
      const [snap, dist] = await Promise.all([
        api.getDashboardSnapshot(),
        api.getSiteDistribution(),
      ]);
      const blob = await drawSnapshotCanvas(snap, dist);
      if (!blob) {
        toast.error(tr('当前浏览器不支持导出 PNG'));
        return;
      }
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      const today = new Date().toISOString().slice(0, 10).replaceAll('-', '');
      a.href = url;
      a.download = `metapi-snapshot-${today}.png`;
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 10_000);
      toast.success(tr('快照已导出'));
    } catch (error: any) {
      toast.error(error?.message || tr('导出快照失败'));
    } finally {
      setExporting(false);
    }
  };

  return (
    <button
      type="button"
      onClick={() => void exportSnapshot()}
      disabled={exporting}
      className="btn btn-ghost"
      style={{ border: '1px solid var(--color-border)', padding: '6px 12px', fontSize: 13 }}
      data-testid="export-snapshot"
    >
      <svg
        width="14"
        height="14"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        style={{ marginRight: 4, verticalAlign: '-2px' }}
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"
        />
      </svg>
      {exporting ? tr('导出中...') : tr('导出快照')}
    </button>
  );
}

/* ------------------------------------------------------------------ */
/*  Canvas drawing                                                      */
/* ------------------------------------------------------------------ */

function roundRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
) {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}

function fmtMoney(v: number): string {
  if (!Number.isFinite(v) || v === 0) return '$0.00';
  if (Math.abs(v) >= 1_000_000) return `$${(v / 1_000_000).toFixed(2)}M`;
  if (Math.abs(v) >= 10_000) return `$${(v / 1_000).toFixed(1)}k`;
  return `$${v.toFixed(2)}`;
}

function fmtNum(v: number): string {
  if (!Number.isFinite(v)) return '0';
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`;
  if (v >= 10_000) return `${(v / 1_000).toFixed(1)}k`;
  return String(Math.round(v));
}

type SnapshotPayload = {
  totalBalance?: number;
  totalUsed?: number;
  todaySpend?: number;
  activeAccounts?: number;
  proxy24h?: { total?: number; success?: number; totalTokens?: number };
  generatedAt?: string;
};

function drawSnapshotCanvas(
  snap: SnapshotPayload,
  dist: { distribution?: Array<{ siteName?: string; totalSpend?: number }> },
): Promise<Blob | null> {
  const canvas = document.createElement('canvas');
  canvas.width = W * 2;
  canvas.height = H * 2;
  const ctx = canvas.getContext('2d');
  if (!ctx) return Promise.resolve(null);
  ctx.scale(2, 2);

  const total = snap.proxy24h?.total ?? 0;
  const success = snap.proxy24h?.success ?? 0;
  const successRate = total > 0 ? (success / total) * 100 : 0;
  const generatedAt = snap.generatedAt
    ? new Date(snap.generatedAt).toLocaleString()
    : new Date().toLocaleString();

  /* background */
  ctx.fillStyle = PALETTE.bg;
  ctx.fillRect(0, 0, W, H);
  const primary = currentAccentPrimary();
  ctx.fillStyle = primary;
  ctx.fillRect(0, 0, W, 6);

  /* header — canvas text is not DOM-reachable by the i18n MutationObserver,
   * so every copy string goes through tr() explicitly */
  ctx.fillStyle = PALETTE.text;
  ctx.font = '700 40px system-ui, -apple-system, "Segoe UI", sans-serif';
  ctx.fillText(tr('MetAPI 网关快照'), 48, 92);
  ctx.fillStyle = PALETTE.muted;
  ctx.font = '400 22px system-ui, -apple-system, "Segoe UI", sans-serif';
  ctx.fillText(`${tr('生成时间：')}${generatedAt}`, 48, 128);

  /* metric cards */
  const metrics = [
    { label: tr('总余额'), value: fmtMoney(snap.totalBalance ?? 0), accent: primary },
    { label: tr('今日消耗'), value: fmtMoney(snap.todaySpend ?? 0), accent: PALETTE.warn },
    { label: tr('24h 请求'), value: fmtNum(total), accent: PALETTE.text },
    { label: tr('24h 成功率'), value: `${successRate.toFixed(1)}%`, accent: total > 0 && successRate < 90 ? PALETTE.danger : PALETTE.success },
    { label: tr('24h Token'), value: fmtNum(snap.proxy24h?.totalTokens ?? 0), accent: PALETTE.text },
    { label: tr('活跃账号'), value: String(snap.activeAccounts ?? 0), accent: PALETTE.text },
  ];
  const cardW = (W - 48 * 2 - 16 * 5) / 6;
  metrics.forEach((m, i) => {
    const x = 48 + i * (cardW + 16);
    ctx.fillStyle = PALETTE.card;
    roundRect(ctx, x, 168, cardW, 148, 14);
    ctx.fill();
    ctx.strokeStyle = PALETTE.border;
    ctx.lineWidth = 1;
    ctx.stroke();
    ctx.fillStyle = PALETTE.muted;
    ctx.font = '400 19px system-ui, -apple-system, "Segoe UI", sans-serif';
    ctx.fillText(m.label, x + 18, 206);
    ctx.fillStyle = m.accent;
    ctx.font = '700 32px system-ui, -apple-system, "Segoe UI", sans-serif';
    ctx.fillText(m.value, x + 18, 258, cardW - 36);
  });

  /* site distribution (top 5 by spend) */
  const sites = (dist.distribution ?? [])
    .filter((s) => s.siteName && Number.isFinite(s.totalSpend) && (s.totalSpend ?? 0) > 0)
    .sort((a, b) => (b.totalSpend ?? 0) - (a.totalSpend ?? 0))
    .slice(0, 5);

  ctx.fillStyle = PALETTE.text;
  ctx.font = '600 24px system-ui, -apple-system, "Segoe UI", sans-serif';
  ctx.fillText(tr('站点消耗 Top'), 48, 384);

  if (sites.length === 0) {
    ctx.fillStyle = PALETTE.muted;
    ctx.font = '400 20px system-ui, -apple-system, "Segoe UI", sans-serif';
    ctx.fillText(tr('暂无站点消耗数据'), 48, 424);
  } else {
    const maxSpend = Math.max(...sites.map((s) => s.totalSpend ?? 0));
    const barMaxW = 900;
    sites.forEach((s, i) => {
      const y = 408 + i * 40;
      const label = String(s.siteName ?? '');
      const spend = s.totalSpend ?? 0;
      const barW = maxSpend > 0 ? Math.max(20, (spend / maxSpend) * barMaxW) : 20;
      ctx.fillStyle = PALETTE.muted;
      ctx.font = '400 19px system-ui, -apple-system, "Segoe UI", sans-serif';
      ctx.fillText(label.length > 28 ? `${label.slice(0, 28)}…` : label, 48, y + 20);
      ctx.fillStyle = PALETTE.border;
      roundRect(ctx, 320, y + 6, barMaxW, 18, 9);
      ctx.fill();
      ctx.fillStyle = primary;
      roundRect(ctx, 320, y + 6, barW, 18, 9);
      ctx.fill();
      ctx.fillStyle = PALETTE.text;
      ctx.font = '600 19px system-ui, -apple-system, "Segoe UI", sans-serif';
      ctx.fillText(fmtMoney(spend), 320 + barMaxW + 18, y + 22);
    });
  }

  /* footer */
  ctx.fillStyle = PALETTE.muted;
  ctx.font = '400 18px system-ui, -apple-system, "Segoe UI", sans-serif';
  ctx.fillText(tr('MetAPI 聚合网关'), 48, 590);

  return new Promise((resolve) => {
    if (typeof canvas.toBlob !== 'function') {
      resolve(null);
      return;
    }
    canvas.toBlob((blob) => resolve(blob), 'image/png');
  });
}
