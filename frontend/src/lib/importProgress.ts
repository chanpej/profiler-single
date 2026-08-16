import type { ImportProgressEvent } from '../types';

export function fileBaseName(path: string): string {
  return path.replace(/^.*[\\/]/, '');
}

export function clampPercent(n: number): number {
  if (!Number.isFinite(n)) return 0;
  return Math.max(0, Math.min(100, Math.round(n)));
}

export function importProgressLabel(p: Pick<ImportProgressEvent, 'phase'>): string {
  if (p.phase === 'read') return '正在读取存档…';
  return '正在解析 Profiler 数据…';
}

export function initialImportProgress(path: string): ImportProgressEvent {
  return {
    phase: 'read',
    done: 0,
    total: 1,
    percent: 0,
    fileName: fileBaseName(path),
  };
}
