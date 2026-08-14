import type { FrameData, ProfilerModuleId } from '../types';

export type SeriesUnit = 'ms' | 'mb' | 'count';

export interface CategoryDef {
  id: string;
  label: string;
  color: string;
  unit: SeriesUnit;
  get: (f: FrameData) => number;
}

export interface ModuleDef {
  id: ProfilerModuleId;
  label: string;
  /** 模块行标题旁的主色，贴近 Unity Profiler 2022。 */
  color: string;
  stacked: boolean;
  categories: CategoryDef[];
  headline: (f: FrameData) => { value: number; unit: SeriesUnit };
}

/** 由 FPS 得到帧间隔（ms）。模块计数器缺失时用来画时间轴。 */
export function frameTimeMs(f: FrameData): number {
  if (!f.fps || f.fps <= 0) return 0;
  return 1000 / f.fps;
}

/** GPU 耗时；缺失时回退到帧间隔，避免 GPU 图空白。 */
export function gpuOrFrameMs(f: FrameData): number {
  if (f.gpuTimeMs > 0) return f.gpuTimeMs;
  return frameTimeMs(f);
}

function cpuKnownMs(f: FrameData): number {
  return (f.cpuRenderingMs || 0) + (f.cpuScriptsMs || 0) + (f.cpuPhysicsMs || 0)
    + (f.cpuAnimationMs || 0) + (f.cpuGcMs || 0) + (f.cpuVsyncMs || 0) + (f.cpuUiMs || 0);
}

/**
 * Others = max(上报的 others, 总 CPU − 已分类耗时)。
 * 分类全 0 时，整段柱画在 Others 上。
 */
export function cpuOthersMs(f: FrameData): number {
  const remainder = (f.cpuTimeMs || 0) - cpuKnownMs(f);
  return Math.max(f.cpuOthersMs || 0, remainder, 0);
}

/** CPU Usage 分类色：对齐 Unity Profiler CPU 模块。 */
export const CPU_CATEGORIES: CategoryDef[] = [
  { id: 'rendering', label: 'Rendering', color: '#2E8B4A', unit: 'ms', get: (f) => f.cpuRenderingMs },
  { id: 'scripts', label: 'Scripts', color: '#3B6CB5', unit: 'ms', get: (f) => f.cpuScriptsMs },
  { id: 'physics', label: 'Physics', color: '#8E44AD', unit: 'ms', get: (f) => f.cpuPhysicsMs },
  { id: 'animation', label: 'Animation', color: '#D35400', unit: 'ms', get: (f) => f.cpuAnimationMs },
  { id: 'gc', label: 'GC', color: '#A0522D', unit: 'ms', get: (f) => f.cpuGcMs },
  { id: 'vsync', label: 'VSync', color: '#7F8C8D', unit: 'ms', get: (f) => f.cpuVsyncMs },
  { id: 'ui', label: 'UI', color: '#C9A227', unit: 'ms', get: (f) => f.cpuUiMs },
  { id: 'others', label: 'Others', color: '#5D6D7E', unit: 'ms', get: (f) => cpuOthersMs(f) },
];

export const MEMORY_CATEGORIES: CategoryDef[] = [
  { id: 'gc', label: 'GC', color: '#E8A838', unit: 'mb', get: (f) => f.gcUsedMB },
  { id: 'gfx', label: 'Gfx', color: '#2E8B4A', unit: 'mb', get: (f) => f.gfxUsedMB },
  { id: 'audio', label: 'Audio', color: '#8E44AD', unit: 'mb', get: (f) => f.audioUsedMB },
  { id: 'video', label: 'Video', color: '#D35400', unit: 'mb', get: (f) => f.videoUsedMB },
  { id: 'profiler', label: 'Profiler', color: '#7F8C8D', unit: 'mb', get: (f) => f.profilerUsedMB },
  { id: 'system', label: 'System', color: '#5D6D7E', unit: 'mb', get: (f) => f.systemUsedMB },
];

export const PROFILER_MODULES: ModuleDef[] = [
  {
    id: 'cpu',
    label: 'CPU Usage',
    color: '#3B6CB5',
    stacked: true,
    categories: CPU_CATEGORIES,
    headline: (f) => ({ value: f.cpuTimeMs, unit: 'ms' }),
  },
  {
    id: 'gpu',
    label: 'GPU Usage',
    color: '#2E8B4A',
    stacked: true,
    categories: [
      { id: 'gpu', label: 'GPU', color: '#2E8B4A', unit: 'ms', get: (f) => gpuOrFrameMs(f) },
    ],
    headline: (f) => ({ value: gpuOrFrameMs(f), unit: 'ms' }),
  },
  {
    id: 'rendering',
    label: 'Rendering',
    color: '#C9A227',
    stacked: false,
    categories: [
      { id: 'drawCalls', label: 'Draw Calls', color: '#3B6CB5', unit: 'count', get: (f) => f.drawCalls },
      { id: 'batches', label: 'Batches', color: '#2E8B4A', unit: 'count', get: (f) => f.batches },
      { id: 'setPass', label: 'SetPass', color: '#D35400', unit: 'count', get: (f) => f.setPassCalls },
      { id: 'shadows', label: 'Shadow Casters', color: '#8E44AD', unit: 'count', get: (f) => f.shadowCasters },
    ],
    headline: (f) => (f.drawCalls > 0
      ? { value: f.drawCalls, unit: 'count' }
      : { value: frameTimeMs(f), unit: 'ms' }),
  },
  {
    id: 'memory',
    label: 'Memory',
    color: '#E8A838',
    stacked: true,
    categories: MEMORY_CATEGORIES,
    headline: (f) => ({ value: f.usedMemMB, unit: 'mb' }),
  },
  {
    id: 'audio',
    label: 'Audio',
    color: '#8E44AD',
    stacked: false,
    categories: [
      { id: 'sources', label: 'Playing Sources', color: '#8E44AD', unit: 'count', get: (f) => f.playingSources },
      { id: 'voices', label: 'Audio Voices', color: '#3B6CB5', unit: 'count', get: (f) => f.audioVoices },
    ],
    headline: (f) => ({ value: f.playingSources, unit: 'count' }),
  },
  {
    id: 'video',
    label: 'Video',
    color: '#D35400',
    stacked: true,
    categories: [
      { id: 'video', label: 'Video', color: '#D35400', unit: 'mb', get: (f) => f.videoUsedMB },
    ],
    headline: (f) => ({ value: f.videoUsedMB, unit: 'mb' }),
  },
  {
    id: 'physics',
    label: 'Physics',
    color: '#8E44AD',
    stacked: false,
    categories: [
      { id: 'bodies', label: 'Active Rigidbodies', color: '#8E44AD', unit: 'count', get: (f) => f.activeRigidbodies },
      { id: 'colliders', label: 'Colliders', color: '#3B6CB5', unit: 'count', get: (f) => f.colliders },
    ],
    headline: (f) => ({ value: f.activeRigidbodies, unit: 'count' }),
  },
  {
    id: 'ui',
    label: 'UI',
    color: '#C9A227',
    stacked: false,
    categories: [
      { id: 'batches', label: 'Batches', color: '#C9A227', unit: 'count', get: (f) => f.uiBatches },
      { id: 'vertices', label: 'Vertices', color: '#3B6CB5', unit: 'count', get: (f) => f.uiVertices },
    ],
    headline: (f) => ({ value: f.uiBatches, unit: 'count' }),
  },
];

export const DEFAULT_ENABLED: ProfilerModuleId[] = ['cpu', 'gpu', 'rendering', 'memory'];

export function moduleById(id: ProfilerModuleId): ModuleDef {
  const m = PROFILER_MODULES.find((x) => x.id === id);
  if (!m) throw new Error(`unknown profiler module: ${id}`);
  return m;
}

export interface SeriesPoint {
  id: string;
  label: string;
  color: string;
  unit: SeriesUnit;
  values: number[];
}

/** 从帧序列提取某个 Profiler 模块的时间轴序列。 */
export function extractModuleSeries(mod: ModuleDef, frames: FrameData[]): SeriesPoint[] {
  const series = mod.categories.map((c) => ({
    id: c.id,
    label: c.label,
    color: c.color,
    unit: c.unit,
    values: frames.map((f) => c.get(f) || 0),
  }));
  if (mod.id === 'gpu' && frames.some((f) => f.gpuTimeMs > 0) === false) {
    return [{
      id: 'frameTime',
      label: 'Frame Time',
      color: mod.color,
      unit: 'ms',
      values: frames.map(frameTimeMs),
    }];
  }
  if (mod.id === 'rendering' && series.every((s) => s.values.every((v) => v === 0))) {
    return [{
      id: 'frameTime',
      label: 'Frame Time',
      color: mod.color,
      unit: 'ms',
      values: frames.map(frameTimeMs),
    }];
  }
  return series;
}

export interface BreakdownRow {
  id: string;
  label: string;
  color: string;
  value: number;
  unit: SeriesUnit;
  pct: number;
}

/** 选中帧在某模块下的分类明细（含占比）。 */
export function moduleBreakdown(mod: ModuleDef, frame: FrameData | null): BreakdownRow[] {
  if (!frame) return [];
  const rows = mod.categories.map((c) => ({
    id: c.id,
    label: c.label,
    color: c.color,
    value: c.get(frame) || 0,
    unit: c.unit,
  }));
  const total = rows.reduce((s, r) => s + r.value, 0);
  return rows.map((r) => ({
    ...r,
    pct: total > 0 ? (r.value / total) * 100 : 0,
  }));
}

/** 计算堆叠图 Y 轴上限：堆叠模块取每帧求和的最大值，否则取各序列峰值。 */
export function chartMax(mod: ModuleDef, series: SeriesPoint[]): number {
  if (series.length === 0) return 1;
  const n = series[0].values.length;
  let peak = 0;
  if (mod.stacked) {
    for (let i = 0; i < n; i++) {
      let sum = 0;
      for (const s of series) sum += s.values[i] || 0;
      if (sum > peak) peak = sum;
    }
  } else {
    for (const s of series) {
      for (const v of s.values) {
        if (v > peak) peak = v;
      }
    }
  }
  if (mod.id === 'cpu' || mod.id === 'gpu') return Math.max(peak * 1.1, 33.4);
  if (peak <= 0) return 1;
  return peak * 1.15;
}

export function memorySimpleRows(frame: FrameData) {
  return [
    { label: 'Total', used: frame.usedMemMB, reserved: frame.reservedMB },
    { label: 'GC', used: frame.gcUsedMB, reserved: frame.gcReservedMB },
    { label: 'GfxDriver', used: frame.gfxUsedMB, reserved: frame.gfxReservedMB },
    { label: 'Audio', used: frame.audioUsedMB, reserved: 0 },
    { label: 'Video', used: frame.videoUsedMB, reserved: 0 },
    { label: 'Profiler', used: frame.profilerUsedMB, reserved: frame.profilerReservedMB },
    { label: 'System', used: frame.systemUsedMB, reserved: 0 },
  ];
}

export function renderingCounters(frame: FrameData) {
  return [
    { label: 'SetPass Calls', value: frame.setPassCalls },
    { label: 'Draw Calls', value: frame.drawCalls },
    { label: 'Total Batches', value: frame.batches },
    { label: 'Triangles', value: frame.triangles },
    { label: 'Vertices', value: frame.vertices },
    { label: 'Shadow Casters', value: frame.shadowCasters },
  ];
}
