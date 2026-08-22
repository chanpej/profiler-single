import { describe, expect, it } from 'vitest';
import { emptyFrame } from './frame';
import {
  CPU_CHART_MAX_Y,
  CPU_CHART_MIN_Y,
  DEFAULT_CPU_CHART_MAX_Y,
  formatChartAxisValue,
  adjustCpuChartMax,
  chartMax,
  extractModuleSeries,
  memoryEditorSummary,
  memoryMetricAvailable,
  memorySimpleRows,
  moduleBreakdown,
  moduleById,
  renderingCounters,
} from './profiler';

function sample() {
  return emptyFrame({
    frameIndex: 10,
    fps: 60,
    cpuTimeMs: 16.4,
    gpuTimeMs: 12,
    cpuRenderingMs: 5,
    cpuScriptsMs: 4,
    cpuPhysicsMs: 2,
    cpuAnimationMs: 1,
    cpuGcMs: 0.5,
    cpuVsyncMs: 1,
    cpuUiMs: 1.5,
    cpuOthersMs: 1.4,
    usedMemMB: 500,
    reservedMB: 700,
    gcUsedMB: 180,
    gcReservedMB: 240,
    gfxUsedMB: 160,
    gfxReservedMB: 200,
    audioUsedMB: 8,
    videoUsedMB: 1,
    profilerUsedMB: 12,
    profilerReservedMB: 20,
    systemUsedMB: 139,
    drawCalls: 300,
    setPassCalls: 40,
    batches: 250,
    triangles: 900000,
    vertices: 600000,
    shadowCasters: 15,
    playingSources: 9,
    audioVoices: 14,
    activeRigidbodies: 42,
    colliders: 100,
    uiBatches: 20,
    uiVertices: 4000,
  });
}

describe('extractModuleSeries', () => {
  it('按模块提取每帧分类值', () => {
    const frames = [sample(), emptyFrame({ cpuScriptsMs: 8, cpuRenderingMs: 2 })];
    const series = extractModuleSeries(moduleById('cpu'), frames);
    const scripts = series.find((s) => s.id === 'scripts');
    expect(scripts?.values).toEqual([4, 8]);
    expect(series).toHaveLength(8);
  });
});

describe('moduleBreakdown', () => {
  it('计算 CPU 分类占比', () => {
    const rows = moduleBreakdown(moduleById('cpu'), sample());
    const rendering = rows.find((r) => r.id === 'rendering');
    expect(rendering?.value).toBe(5);
    const sum = rows.reduce((s, r) => s + r.pct, 0);
    expect(sum).toBeCloseTo(100, 5);
  });
  it('空帧返回空数组', () => {
    expect(moduleBreakdown(moduleById('cpu'), null)).toEqual([]);
  });
});

describe('chartMax', () => {
  it('CPU 堆叠上限至少 33.4ms', () => {
    const frames = [sample()];
    const series = extractModuleSeries(moduleById('cpu'), frames);
    expect(chartMax(moduleById('cpu'), series)).toBeGreaterThanOrEqual(33.4);
  });
  it('Rendering 固定使用 100 的纵轴上限', () => {
    const frames = [emptyFrame({ drawCalls: 100, batches: 80, setPassCalls: 20, shadowCasters: 5 })];
    const series = extractModuleSeries(moduleById('rendering'), frames);
    expect(chartMax(moduleById('rendering'), series)).toBe(100);
  });
  it('GPU Usage 与 Rendering 固定使用 100 作为纵轴上限', () => {
    const gpu = extractModuleSeries(moduleById('gpu'), [emptyFrame({ gpuTimeMs: 12 })]);
    const rendering = extractModuleSeries(moduleById('rendering'), [emptyFrame({ drawCalls: 20 })]);
    expect(chartMax(moduleById('gpu'), gpu)).toBe(100);
    expect(chartMax(moduleById('rendering'), rendering)).toBe(100);

    const gpuSpike = extractModuleSeries(moduleById('gpu'), [emptyFrame({ gpuTimeMs: 160 })]);
    const renderingSpike = extractModuleSeries(moduleById('rendering'), [emptyFrame({ drawCalls: 160 })]);
    expect(chartMax(moduleById('gpu'), gpuSpike)).toBe(100);
    expect(chartMax(moduleById('rendering'), renderingSpike)).toBe(100);
  });
});

describe('adjustCpuChartMax', () => {
  it('向下拖动扩大正向上限，向上拖动缩小，且不越过零', () => {
    expect(adjustCpuChartMax(20, 100)).toBeGreaterThan(20);
    expect(adjustCpuChartMax(20, -100)).toBeLessThan(20);
    expect(adjustCpuChartMax(1, -10_000)).toBe(CPU_CHART_MIN_Y);
    expect(adjustCpuChartMax(CPU_CHART_MAX_Y, 10_000)).toBe(CPU_CHART_MAX_Y);
  });

  it('默认上限为 100ms', () => {
    expect(DEFAULT_CPU_CHART_MAX_Y).toBe(100);
  });
});

describe('formatChartAxisValue', () => {
  it('纵轴标签使用无单位的非负整数', () => {
    expect(formatChartAxisValue(16.7)).toBe('17');
    expect(formatChartAxisValue(327.71)).toBe('328');
    expect(formatChartAxisValue(-5)).toBe('0');
  });
});

describe('memorySimpleRows', () => {
  it('按 Unity 计数器全名列出 Total、GC、Gfx 等 Simple 项目', () => {
    const labels = memorySimpleRows(sample()).map((r) => r.label);
    expect(labels).toEqual([
      'Total Used Memory', 'Total Reserved Memory', 'GC Used Memory', 'GC Reserved Memory',
      'Gfx Used Memory', 'Gfx Reserved Memory', 'Audio Used Memory', 'Video Used Memory',
      'Profiler Used Memory', 'Profiler Reserved Memory',
    ]);
  });

  it('按 Editor 定义计算 committed、untracked 与 Other', () => {
    const frame = sample();
    frame.committedMemMB = 700;
    const memory = memoryEditorSummary(frame);
    expect(memory.untracked).toBe(0);
    expect(memory.breakdown.find((row) => row.id === 'other')?.used).toBe(139);
  });

  it('计数器掩码将未采集值与实际的零值区分开', () => {
    const partial = emptyFrame({ memoryCounterMask: 1 << 6, audioUsedMB: 3.5 });
    expect(memoryMetricAvailable(partial, 'audioUsed')).toBe(true);
    expect(memoryMetricAvailable(partial, 'totalUsed')).toBe(false);
    expect(memoryMetricAvailable(emptyFrame(), 'totalUsed')).toBe(true);
  });
});

describe('renderingCounters', () => {
  it('列出 DrawCall / SetPass / 三角形等计数器', () => {
    const rows = renderingCounters(sample());
    expect(rows.find((r) => r.label === 'Draw Calls')?.value).toBe(300);
    expect(rows.find((r) => r.label === 'Triangles')?.value).toBe(900000);
  });
});

describe('分类缺失帧（无 Unity 模块计数器）', () => {
  const coarseFrame = emptyFrame({
    frameIndex: 2086,
    fps: 59.9,
    cpuTimeMs: 34.3,
    usedMemMB: 700.2,
    gfxUsedMB: 80,
    systemUsedMB: 226,
  });

  it('CPU 图把总耗时画到 Others，避免空白', () => {
    const series = extractModuleSeries(moduleById('cpu'), [coarseFrame]);
    const others = series.find((s) => s.id === 'others');
    expect(others?.values[0]).toBeCloseTo(34.3);
    const stacked = series.reduce((s, r) => s + r.values[0], 0);
    expect(stacked).toBeCloseTo(34.3);
    const rows = moduleBreakdown(moduleById('cpu'), coarseFrame);
    expect(rows.find((r) => r.id === 'others')?.pct).toBeCloseTo(100);
  });

  it('GPU 图在无 GPU 数据时回退到 Frame Time', () => {
    const series = extractModuleSeries(moduleById('gpu'), [coarseFrame]);
    expect(series[0].id).toBe('frameTime');
    expect(series[0].values[0]).toBeCloseTo(1000 / 59.9, 1);
  });

  it('Rendering 图在无 DrawCall 时回退到 Frame Time', () => {
    const series = extractModuleSeries(moduleById('rendering'), [coarseFrame]);
    expect(series[0].label).toBe('Frame Time');
    expect(series[0].values[0]).toBeCloseTo(1000 / 59.9, 1);
    expect(moduleById('rendering').headline(coarseFrame).unit).toBe('ms');
  });

  it('完整分类数据不受回退影响', () => {
    const series = extractModuleSeries(moduleById('cpu'), [sample()]);
    expect(series.find((s) => s.id === 'others')?.values[0]).toBeCloseTo(1.4);
    expect(extractModuleSeries(moduleById('gpu'), [sample()])[0].id).toBe('gpu');
    expect(extractModuleSeries(moduleById('rendering'), [sample()])[0].id).toBe('drawCalls');
  });
});
