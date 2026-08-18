import { describe, expect, it } from 'vitest';
import { clampPercent, fileBaseName, importProgressLabel, initialImportProgress } from './importProgress';

describe('importProgress', () => {
  it('提取 Windows / POSIX 文件名', () => {
    expect(fileBaseName('D:\\captures\\boss.data')).toBe('boss.data');
    expect(fileBaseName('/tmp/session.data')).toBe('session.data');
    expect(fileBaseName('plain.data')).toBe('plain.data');
  });

  it('百分比夹紧到 0..100', () => {
    expect(clampPercent(-3)).toBe(0);
    expect(clampPercent(42.4)).toBe(42);
    expect(clampPercent(150)).toBe(100);
    expect(clampPercent(Number.NaN)).toBe(0);
  });

  it('按阶段给出提示文案', () => {
    expect(importProgressLabel({ phase: 'read' })).toBe('正在读取存档…');
    expect(importProgressLabel({ phase: 'parse' })).toBe('正在解析 Profiler 数据…');
  });

  it('加载开始时进度为 0', () => {
    const p = initialImportProgress('C:\\data\\fight.data');
    expect(p.fileName).toBe('fight.data');
    expect(p.phase).toBe('read');
    expect(p.percent).toBe(0);
  });
});
