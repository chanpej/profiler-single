import type { FrameData, ProfilerModuleId } from '../types';

export type SeriesUnit = 'ms' | 'mb' | 'kb' | 'count';

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

// 与后端 protocol.MemoryCounter* 常量一致。掩码缺省代表旧采集，数值仍按可用显示。
export const MEMORY_COUNTER_BITS = {
  totalUsed: 1 << 0,
  totalReserved: 1 << 1,
  gcUsed: 1 << 2,
  gcReserved: 1 << 3,
  gfxUsed: 1 << 4,
  gfxReserved: 1 << 5,
  audioUsed: 1 << 6,
  videoUsed: 1 << 7,
  profilerUsed: 1 << 8,
  profilerReserved: 1 << 9,
  systemUsed: 1 << 10,
  committed: 1 << 11,
  texture: 1 << 12,
  mesh: 1 << 13,
  materialCount: 1 << 14,
  objectCount: 1 << 15,
  textureCount: 1 << 16,
  meshCount: 1 << 17,
  gameObjectCount: 1 << 18,
  sceneObjectCount: 1 << 19,
  material: 1 << 20,
  animationClip: 1 << 21,
  animationClipCount: 1 << 22,
  assetCount: 1 << 23,
  gcAllocationFrameCount: 1 << 24,
  gcAllocatedFrame: 1 << 25,
} as const;

// Unity Editor Memory 模块顶部折线图的指标，不是 Simple 视图的 Breakdown。
export const MEMORY_CATEGORIES: CategoryDef[] = [
  { id: 'total', label: 'Total Memory', color: '#E8A838', unit: 'mb', get: (f) => f.usedMemMB },
  { id: 'texture', label: 'Texture Memory', color: '#37A8C9', unit: 'mb', get: (f) => f.textureMemMB },
  { id: 'mesh', label: 'Mesh Memory', color: '#55B97C', unit: 'mb', get: (f) => f.meshMemMB },
  { id: 'materialCount', label: 'Material Count', color: '#A86CD5', unit: 'count', get: (f) => f.materialCount },
  { id: 'objectCount', label: 'Object Count', color: '#D35400', unit: 'count', get: (f) => f.objectCount },
  { id: 'gc', label: 'GC Used Memory', color: '#E05D44', unit: 'mb', get: (f) => f.gcUsedMB },
  { id: 'audio', label: 'Audio Used Memory', color: '#58B98A', unit: 'mb', get: (f) => f.audioUsedMB },
  { id: 'gcAlloc', label: 'GC Allocated In Frame', color: '#C0C6D1', unit: 'kb', get: (f) => f.gcAllocatedFrameKB || f.gcAllocKB },
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
    // Memory 是不同单位的独立折线，不能按分类堆叠。
    stacked: false,
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

export interface MemoryMetric {
  id: keyof typeof MEMORY_COUNTER_BITS;
  label: string;
  value: number;
}

/** 旧采集没有掩码时保留既有行为；新采集可明确区分“0”和“未采集”。 */
export function memoryMetricAvailable(frame: FrameData, id: keyof typeof MEMORY_COUNTER_BITS): boolean {
  return frame.memoryCounterMask == null || (frame.memoryCounterMask & MEMORY_COUNTER_BITS[id]) !== 0;
}

export interface MemoryBreakdownRow {
  id: string;
  label: string;
  color: string;
  used: number;
  reserved?: number;
  available: boolean;
}

/** 将 Player 的原始 Memory 计数器归并成 Unity Editor Simple 视图的语义。 */
export function memoryEditorSummary(frame: FrameData) {
  const available = (id: keyof typeof MEMORY_COUNTER_BITS) => memoryMetricAvailable(frame, id);
  const committed = available('committed') ? frame.committedMemMB
    : (available('systemUsed') ? frame.systemUsedMB : 0);
  const committedAvailable = available('committed') || available('systemUsed');
  const trackedAvailable = available('totalUsed') && available('totalReserved');
  const untracked = committedAvailable && available('totalReserved')
    ? Math.max(committed - frame.reservedMB, 0)
    : 0;
  const knownUsed = frame.gcUsedMB + frame.gfxUsedMB + frame.audioUsedMB + frame.videoUsedMB + frame.profilerUsedMB;
  const knownReserved = frame.gcReservedMB + frame.gfxReservedMB + frame.profilerReservedMB;
  const breakdown: MemoryBreakdownRow[] = [
    { id: 'managed', label: 'Managed Heap', color: '#8AA32D', used: frame.gcUsedMB, reserved: frame.gcReservedMB, available: available('gcUsed') },
    { id: 'graphics', label: 'Graphics & Graphics Driver', color: '#35A8C8', used: frame.gfxUsedMB, reserved: frame.gfxReservedMB, available: available('gfxUsed') },
    { id: 'audio', label: 'Audio', color: '#58B98A', used: frame.audioUsedMB, available: available('audioUsed') },
    { id: 'video', label: 'Video', color: '#D7B43E', used: frame.videoUsedMB, available: available('videoUsed') },
    {
      id: 'other', label: 'Other', color: '#76808C',
      used: trackedAvailable ? Math.max(frame.usedMemMB - knownUsed, 0) : 0,
      reserved: trackedAvailable ? Math.max(frame.reservedMB - knownReserved, 0) : undefined,
      available: trackedAvailable,
    },
    { id: 'profiler', label: 'Profiler', color: '#4D9B88', used: frame.profilerUsedMB, reserved: frame.profilerReservedMB, available: available('profilerUsed') },
  ];
  return {
    committed,
    committedAvailable,
    trackedUsed: frame.usedMemMB,
    trackedReserved: frame.reservedMB,
    trackedAvailable,
    untracked,
    untrackedAvailable: committedAvailable && available('totalReserved'),
    breakdown,
  };
}

/** Unity Memory Simple 页签中可由 Player Profiler 计数器提供的原始项目。 */
export function memorySimpleRows(frame: FrameData): MemoryMetric[] {
  return [
    { id: 'totalUsed', label: 'Total Used Memory', value: frame.usedMemMB },
    { id: 'totalReserved', label: 'Total Reserved Memory', value: frame.reservedMB },
    { id: 'gcUsed', label: 'GC Used Memory', value: frame.gcUsedMB },
    { id: 'gcReserved', label: 'GC Reserved Memory', value: frame.gcReservedMB },
    { id: 'gfxUsed', label: 'Gfx Used Memory', value: frame.gfxUsedMB },
    { id: 'gfxReserved', label: 'Gfx Reserved Memory', value: frame.gfxReservedMB },
    { id: 'audioUsed', label: 'Audio Used Memory', value: frame.audioUsedMB },
    { id: 'videoUsed', label: 'Video Used Memory', value: frame.videoUsedMB },
    { id: 'profilerUsed', label: 'Profiler Used Memory', value: frame.profilerUsedMB },
    { id: 'profilerReserved', label: 'Profiler Reserved Memory', value: frame.profilerReservedMB },
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
