import type { SampleNode } from '../types';

/** Wails 绑定里 Go 切片可能是 null，归一化后再交给 UI。 */
export interface WireSampleNode {
  name: string;
  category: string;
  totalMs: number;
  selfMs: number;
  calls: number;
  gcAllocKB: number;
  startMs?: number | null;
  callstack?: string[] | null;
  children?: WireSampleNode[] | null;
}

/** 把绑定层采样树转成前端 SampleNode（去掉 null）。 */
export function normalizeSampleTree(rows: WireSampleNode[] | null | undefined): SampleNode[] {
  if (!rows) return [];
  return rows.map(normalizeSample);
}

function normalizeSample(n: WireSampleNode): SampleNode {
  return {
    name: n.name,
    category: n.category,
    totalMs: n.totalMs,
    selfMs: n.selfMs,
    calls: n.calls,
    gcAllocKB: n.gcAllocKB,
    startMs: n.startMs ?? undefined,
    callstack: n.callstack ?? undefined,
    children: n.children ? normalizeSampleTree(n.children) : undefined,
  };
}
