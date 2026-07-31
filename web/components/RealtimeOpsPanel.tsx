import React, { useEffect, useRef, useState } from 'react';

import { getAuthToken } from '../authSession.js';
import { tr } from '../i18n.js';

interface RealtimePoint {
  ts: number;
  total: number;
  success: number;
}

interface RealtimeFrame {
  lifetime: number;
  points: RealtimePoint[];
}

/**
 * B2 (sub2api/cliproxyapi borrow): live ops traffic panel.
 * Opens a WebSocket to /api/admin/ops/ws?token=… (browser WS cannot send the
 * Authorization header, so the token travels as a query param — same value the
 * admin API middleware checks). One frame per second: current QPS, success
 * rate and a 60s sparkline. This instance's traffic only (multi-instance
 * honesty). Auto-reconnects with backoff; silently hides when no token.
 */
export default function RealtimeOpsPanel() {
  const [qps, setQps] = useState(0);
  const [successRate, setSuccessRate] = useState<number | null>(null);
  const [lifetime, setLifetime] = useState(0);
  const [spark, setSpark] = useState<number[]>([]);
  const [connected, setConnected] = useState(false);
  const [gaveUp, setGaveUp] = useState(false);
  const socketRef = useRef<WebSocket | null>(null);
  const retryRef = useRef(0);
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    const token = getAuthToken(localStorage);
    if (!token) return;

    let disposed = false;
    // A 403 (token rotated/revoked) must not retry forever: after MAX_FAILS
    // consecutive failures the panel gives up and shows the error state.
    const MAX_FAILS = 5;
    let fails = 0;
    const loc = typeof window !== 'undefined' ? window.location : null;
    const protocol = loc && loc.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = loc ? loc.host : 'localhost';
    const connect = () => {
      if (disposed) return;
      try {
        const ws = new WebSocket(
          `${protocol}//${host}/api/admin/ops/ws?token=${encodeURIComponent(token)}`,
        );
        socketRef.current = ws;

        ws.onopen = () => {
          retryRef.current = 0;
          fails = 0;
          setGaveUp(false);
          setConnected(true);
        };
        ws.onmessage = (ev) => {
          try {
            const frame = JSON.parse(ev.data as string) as RealtimeFrame;
            setLifetime(frame.lifetime ?? 0);
            const pts = Array.isArray(frame.points) ? frame.points : [];
            if (pts.length > 0) {
              const last = pts[pts.length - 1];
              setQps(last.total);
              setSuccessRate(last.total > 0 ? (last.success / last.total) * 100 : null);
            }
            setSpark(pts.slice(-60).map((p) => p.total));
          } catch {
            // ignore malformed frame
          }
        };
        ws.onclose = () => {
          setConnected(false);
          if (disposed) return;
          fails += 1;
          if (fails >= MAX_FAILS) {
            setGaveUp(true);
            return;
          }
          const delay = Math.min(1000 * 2 ** retryRef.current, 15000);
          retryRef.current += 1;
          timerRef.current = window.setTimeout(connect, delay);
        };
        ws.onerror = () => {
          ws.close();
        };
      } catch {
        // WS construction failure — retry later
        timerRef.current = window.setTimeout(connect, 5000);
      }
    };

    connect();
    return () => {
      disposed = true;
      if (timerRef.current != null) window.clearTimeout(timerRef.current);
      socketRef.current?.close();
    };
  }, []);

  // Simple zero-dependency bar sparkline (fixed 60 columns).
  const maxSpark = Math.max(...spark, 1);
  const bars = spark.map((v, i) => (
    <div
      key={i}
      title={`${v} req/s`}
      style={{
        flex: 1,
        minWidth: 2,
        background:
          v > 0 ? 'var(--color-primary, #1a73e8)' : 'var(--color-border-light)',
        height: `${Math.max(6, Math.round((v / maxSpark) * 100))}%`,
        borderRadius: 1,
        alignSelf: 'flex-end',
        opacity: v > 0 ? 0.85 : 0.4,
      }}
    />
  ));

  return (
    <div className="card chart-panel-enter animate-slide-up stagger-4" style={{ padding: 16 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10, flexWrap: 'wrap', gap: 8 }}>
        <div>
          <span style={{ fontWeight: 600, fontSize: 14 }}>{tr('实时流量')}</span>
          <span
            className="badge"
            style={{
              marginLeft: 8,
              fontSize: 11,
              background: connected ? 'rgba(52,168,83,0.15)' : gaveUp ? 'rgba(217,48,37,0.15)' : 'rgba(234,67,53,0.15)',
              color: connected ? '#34a853' : gaveUp ? '#d93025' : '#ea4335',
            }}
          >
            {connected ? tr('在线') : gaveUp ? tr('已断开（鉴权失败）') : tr('重连中…')}
          </span>
        </div>
        <span style={{ fontSize: 12, color: 'var(--color-text-muted)' }}>
          {tr('累计请求')} <b style={{ color: 'var(--color-text-primary)' }}>{lifetime.toLocaleString()}</b>
        </span>
      </div>
      <div style={{ display: 'flex', gap: 24, alignItems: 'center', flexWrap: 'wrap' }}>
        <div>
          <div style={{ fontSize: 26, fontWeight: 700, lineHeight: 1.2 }}>{qps}</div>
          <div style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{tr('当前 QPS')}</div>
        </div>
        <div>
          <div
            style={{
              fontSize: 26,
              fontWeight: 700,
              lineHeight: 1.2,
              color:
                successRate == null
                  ? 'var(--color-text-muted)'
                  : successRate >= 95
                    ? 'var(--color-success, #34a853)'
                    : 'var(--color-danger, #ea4335)',
            }}
          >
            {successRate == null ? '—' : `${successRate.toFixed(1)}%`}
          </div>
          <div style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>{tr('成功率（近 1s）')}</div>
        </div>
        <div
          style={{
            flex: 1,
            minWidth: 220,
            height: 44,
            display: 'flex',
            alignItems: 'flex-end',
            gap: 2,
            borderBottom: '1px solid var(--color-border-light)',
          }}
        >
          {bars}
        </div>
      </div>
      <div style={{ fontSize: 11, color: 'var(--color-text-muted)', marginTop: 8 }}>
        {tr('近 60 秒本实例流量（每秒）——多实例下每实例独立计数')}
      </div>
    </div>
  );
}
