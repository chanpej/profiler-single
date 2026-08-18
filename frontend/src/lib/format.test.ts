import { describe, expect, it } from 'vitest';
import { formatClock, formatCount, formatFps, formatGCAlloc, formatKB, formatMB, formatMB1, formatMs, formatPct, formatProfilerValue, fpsLevel } from './format';

describe('formatMs', () => {
  it('保留 2 位小数并带单位', () => {
    expect(formatMs(16.66)).toBe('16.66ms');
    expect(formatMs(0)).toBe('0.00ms');
  });
  it('非法值返回占位符', () => {
    expect(formatMs(NaN)).toBe('--');
    expect(formatMs(Infinity)).toBe('--');
  });
});

describe('formatFps', () => {
  it('正常格式化', () => {
    expect(formatFps(59.94)).toBe('59.9');
  });
  it('0 或负值返回占位符', () => {
    expect(formatFps(0)).toBe('--');
    expect(formatFps(-1)).toBe('--');
    expect(formatFps(NaN)).toBe('--');
  });
});

describe('formatMB', () => {
  it('小于 1024 显示 MB', () => {
    expect(formatMB(512.4)).toBe('512MB');
  });
  it('大于等于 1024 转 GB', () => {
    expect(formatMB(2048)).toBe('2.00GB');
  });
  it('负值返回占位符', () => {
    expect(formatMB(-5)).toBe('--');
  });
});

describe('formatKB', () => {
  it('小于 1024 显示 KB', () => {
    expect(formatKB(128)).toBe('128KB');
  });
  it('大于等于 1024 转 MB', () => {
    expect(formatKB(2560)).toBe('2.5MB');
  });
});

describe('formatCount', () => {
  it('缩写大数字', () => {
    expect(formatCount(999)).toBe('999');
    expect(formatCount(1200)).toBe('1.2k');
    expect(formatCount(3400000)).toBe('3.4M');
  });
});

describe('fpsLevel', () => {
  it('按阈值分级', () => {
    expect(fpsLevel(60)).toBe('good');
    expect(fpsLevel(50)).toBe('good');
    expect(fpsLevel(45)).toBe('warn');
    expect(fpsLevel(30)).toBe('warn');
    expect(fpsLevel(20)).toBe('bad');
  });
});

describe('formatMB1', () => {
  it('保留 1 位小数', () => {
    expect(formatMB1(512.44)).toBe('512.4 MB');
    expect(formatMB1(2048)).toBe('2.00 GB');
  });
});

describe('formatGCAlloc', () => {
  it('按 B / KB / MB 切换并固定保留 1 位小数', () => {
    expect(formatGCAlloc(0)).toBe('0.0 B');
    expect(formatGCAlloc(0.5)).toBe('512.0 B');
    expect(formatGCAlloc(12.34)).toBe('12.3 KB');
    expect(formatGCAlloc(1230.6)).toBe('1.2 MB');
  });

  it('非法值返回占位符', () => {
    expect(formatGCAlloc(-1)).toBe('--');
    expect(formatGCAlloc(NaN)).toBe('--');
  });
});

describe('formatPct', () => {
  it('保留 1 位小数并带百分号', () => {
    expect(formatPct(12.34)).toBe('12.3%');
    expect(formatPct(NaN)).toBe('--');
  });
});

describe('formatProfilerValue', () => {
  it('按单位分发', () => {
    expect(formatProfilerValue(16.66, 'ms')).toBe('16.66ms');
    expect(formatProfilerValue(12.3, 'mb')).toBe('12.3 MB');
    expect(formatProfilerValue(1500, 'count')).toBe('1.5k');
  });
});

describe('formatClock', () => {
  it('输出 HH:MM:SS', () => {
    expect(formatClock(new Date(2026, 0, 1, 9, 5, 3).getTime())).toBe('09:05:03');
  });
  it('非法时间戳返回占位符', () => {
    expect(formatClock(0)).toBe('--');
    expect(formatClock(NaN)).toBe('--');
  });
});
