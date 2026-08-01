import { describe, expect, it } from 'vitest';
import { translateText } from './i18n.js';

describe('translateText', () => {
  it('keeps zh text unchanged in zh mode', () => {
    expect(translateText('模型广场', 'zh')).toBe('模型广场');
  });

  it('translates exact key in en mode', () => {
    expect(translateText('模型广场', 'en')).toBe('Model Marketplace');
  });

  it('supports phrase replacement for mixed text', () => {
    expect(translateText('覆盖槽位 3', 'en')).toBe('Coverage Slots 3');
    expect(translateText('共 12 个模型', 'en')).toBe('Total 12 models');
  });

  it('never returns Chinese characters in strict en mode', () => {
    const samples = [
      '站点已禁用',
      '缓存清理后重建失败：unknown error',
      '签到任务执行中，请稍后查看签到日志',
    ];

    for (const sample of samples) {
      expect(translateText(sample, 'en')).not.toMatch(/[\u3400-\u9fff]/);
    }
  });

  it('uses concrete english translations instead of fallback for common runtime text', () => {
    expect(translateText('切换到中文', 'en')).toBe('Switch to Chinese');
    expect(translateText('中', 'en')).toBe('ZH');

    const samples = [
      '站点已禁用',
      '签到任务执行中，请稍后查看签到日志',
      '下游访问令牌至少 6 位（含 sk-）',
      '路由重建任务执行中，请稍后查看程序日志',
    ];

    for (const sample of samples) {
      const translated = translateText(sample, 'en');
      expect(translated).not.toBe('Untranslated');
      expect(translated).not.toMatch(/[\u3400-\u9fff]/);
    }
  });
});

describe('translateText review-wave regressions (2026-08-01)', () => {
  it('tr() button/toast copy has exact keys (was Untranslated in EN)', () => {
    expect(translateText('导出快照', 'en')).toBe('Export Snapshot');
    expect(translateText('导出中...', 'en')).toBe('Exporting...');
    expect(translateText('快照已导出', 'en')).toBe('Snapshot exported');
    expect(translateText('导出快照失败', 'en')).toBe('Failed to export snapshot');
    expect(translateText('当前浏览器不支持导出 PNG', 'en')).toBe('PNG export is not supported in this browser');
    expect(translateText('实时流量', 'en')).toBe('Live traffic');
    expect(translateText('详情', 'en')).toBe('Details');
    expect(translateText('不确定', 'en')).toBe('Inconclusive');
  });

  it('single-char keys never shred words they appear inside', () => {
    // '中' is isolated inside the word → untouched; exact keys win.
    expect(translateText('登录中...', 'en')).toBe('Signing in...');
    expect(translateText('模型同步中', 'en')).toBe('Syncing models');
    expect(translateText('导出中...', 'en')).not.toContain('ZH');
    // '共' stands alone (non-Han neighbours) → replaced.
    expect(translateText('共 12 个模型', 'en')).toBe('Total 12 models');
    // standalone single char still translates.
    expect(translateText('中', 'en')).toBe('ZH');
  });

  it('trim-exact lookup covers runtime nodes with JSX whitespace', () => {
    expect(translateText('条，共 ', 'en')).toBe(' of ');
    expect(translateText(' 个凭证', 'en')).toBe('credentials');
    expect(translateText('显示第 ', 'en')).toBe('Showing ');
  });

  it('interpolated chart title fragments translate independently', () => {
    expect(translateText('模型成本分布（近 ', 'en')).toBe('Model cost distribution (last ');
    expect(translateText('延迟直方图（近 ', 'en')).toBe('Latency histogram (last ');
    expect(translateText('P95 采样截断（', 'en')).toBe('P95 truncated (');
    expect(translateText('天）', 'en')).toBe(' days)');
    expect(translateText('余额趋势（近 ', 'en')).toBe('Balance trend (last ');
  });

  it('dictionary error fixes (M1-M4)', () => {
    expect(translateText('跳过', 'en')).toBe('Skipped');
    expect(translateText('重试', 'en')).toBe('Retry');
    expect(translateText('豆包', 'en')).toBe('Doubao');
    expect(translateText('路由不存在', 'en')).toBe('Route not found');
    expect(translateText('通道', 'en')).toBe('Channel');
    expect(translateText('消耗趋势', 'en')).toBe('Spend trend');
    expect(translateText('调用趋势', 'en')).toBe('Calls trend');
  });
});
