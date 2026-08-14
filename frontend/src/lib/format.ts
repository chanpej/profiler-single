// 纯格式化工具函数，供组件展示数值使用。

/** 格式化毫秒耗时，如 16.7ms。 */
export function formatMs(v: number): string {
  if (!Number.isFinite(v)) return '--';
  return `${v.toFixed(1)}ms`;
}

/** 格式化 FPS，保留 1 位小数。 */
export function formatFps(v: number): string {
  if (!Number.isFinite(v) || v <= 0) return '--';
  return v.toFixed(1);
}

/** 格式化内存（MB），大于 1024 时转为 GB。 */
export function formatMB(v: number): string {
  if (!Number.isFinite(v) || v < 0) return '--';
  if (v >= 1024) return `${(v / 1024).toFixed(2)}GB`;
  return `${v.toFixed(0)}MB`;
}

/** 格式化 KB，大于 1024 时转为 MB。 */
export function formatKB(v: number): string {
  if (!Number.isFinite(v) || v < 0) return '--';
  if (v >= 1024) return `${(v / 1024).toFixed(1)}MB`;
  return `${v.toFixed(0)}KB`;
}

/** 大数字缩写：1.2k / 3.4M。 */
export function formatCount(v: number): string {
  if (!Number.isFinite(v)) return '--';
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`;
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}k`;
  return `${Math.round(v)}`;
}

/** 根据 FPS 返回状态色级别。 */
export function fpsLevel(fps: number): 'good' | 'warn' | 'bad' {
  if (fps >= 50) return 'good';
  if (fps >= 30) return 'warn';
  return 'bad';
}

/** 格式化内存（MB，保留 1 位小数），贴近 Unity Profiler Memory 模块。 */
export function formatMB1(v: number): string {
  if (!Number.isFinite(v) || v < 0) return '--';
  if (v >= 1024) return `${(v / 1024).toFixed(2)} GB`;
  return `${v.toFixed(1)} MB`;
}

/** 格式化百分比。 */
export function formatPct(v: number): string {
  if (!Number.isFinite(v)) return '--';
  return `${v.toFixed(1)}%`;
}

/** 按单位格式化 Profiler 数值。 */
export function formatProfilerValue(v: number, unit: 'ms' | 'mb' | 'count'): string {
  if (unit === 'ms') return formatMs(v);
  if (unit === 'mb') return formatMB1(v);
  return formatCount(v);
}

/** 时间戳(ms) → HH:MM:SS。 */
export function formatClock(timestampMs: number): string {
  if (!Number.isFinite(timestampMs) || timestampMs <= 0) return '--';
  const d = new Date(timestampMs);
  const p = (n: number) => String(n).padStart(2, '0');
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}
