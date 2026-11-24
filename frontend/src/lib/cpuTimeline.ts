import type { CpuThreadSamples, CpuTimelineFrame, FrameData, SampleNode } from '../types';
import { categoryColor } from './hierarchy';

/** 左侧线程名列宽，贴近 Unity Profiler Timeline。 */
export const THREAD_LABEL_W = 108;
/** 顶部时间刻度条高度。 */
export const RULER_H = 18;
export const BAR_H = 16;
export const BAR_GAP = 1;
export const THREAD_PAD = 4;
export const THREAD_GAP = 6;
/** 最短可见窗口，避免滚轮缩进到看不见。 */
export const MIN_SPAN_MS = 0.2;
const ZOOM_IN = 1 / 1.25;
const ZOOM_OUT = 1.25;

export interface TimelineBar {
  id: string;
  path: string;
  samplePath?: string[];
  name: string;
  category: string;
  color: string;
  startMs: number;
  durationMs: number;
  selfMs?: number;
  calls?: number;
  callstack?: string[];
  depth: number;
  thread: string;
  ghost?: boolean;
}

export interface TimelineThread {
  id: string;
  name: string;
  bars: TimelineBar[];
  maxDepth: number;
}

export interface TimeWindow {
  /** 可见窗口左缘（ms，相对本帧 0）。 */
  startMs: number;
  /** 可见时长（ms）。 */
  spanMs: number;
}

export interface TimelineLayout {
  threads: TimelineThread[];
  startMs: number;
  totalMs: number;
}

export function barEnd(bar: TimelineBar): number {
  return bar.startMs + bar.durationMs;
}

export function windowEnd(win: TimeWindow): number {
  return win.startMs + win.spanMs;
}

export function fitWindow(totalMs: number, startMs = 0): TimeWindow {
  return { startMs, spanMs: Math.max(totalMs - startMs, MIN_SPAN_MS) };
}

export function clampWindow(win: TimeWindow, totalMs: number, minimum = 0): TimeWindow {
  const maxSpan = Math.max(totalMs - minimum, MIN_SPAN_MS);
  const spanMs = Math.min(maxSpan, Math.max(MIN_SPAN_MS, win.spanMs));
  const maxStart = minimum + maxSpan - spanMs;
  const startMs = Math.min(maxStart, Math.max(minimum, win.startMs));
  return { startMs, spanMs };
}

/**
 * 围绕 anchorMs 缩放。factor < 1 放大（更短窗口），> 1 缩小。
 */
export function zoomWindow(win: TimeWindow, totalMs: number, factor: number, anchorMs: number, minimum = 0): TimeWindow {
  const spanMs = win.spanMs * factor;
  const ratio = win.spanMs <= 0 ? 0 : (anchorMs - win.startMs) / win.spanMs;
  const startMs = anchorMs - ratio * spanMs;
  return clampWindow({ startMs, spanMs }, totalMs, minimum);
}

export function panWindow(win: TimeWindow, totalMs: number, deltaMs: number, minimum = 0): TimeWindow {
  return clampWindow({ ...win, startMs: win.startMs + deltaMs }, totalMs, minimum);
}

/** 双击采样：把该条放到视野中央并略留边。 */
export function zoomToBar(bar: TimelineBar, totalMs: number, minimum = 0): TimeWindow {
  const pad = Math.max(bar.durationMs * 0.2, MIN_SPAN_MS * 0.5);
  const spanMs = Math.max(bar.durationMs + pad * 2, MIN_SPAN_MS);
  return clampWindow({ startMs: bar.startMs - pad, spanMs }, totalMs, minimum);
}

export function wheelZoomFactor(deltaY: number): number {
  if (deltaY === 0) return 1;
  return deltaY > 0 ? ZOOM_OUT : ZOOM_IN;
}

export function msFromX(x: number, plotW: number, win: TimeWindow): number {
  if (plotW <= 0) return win.startMs;
  return win.startMs + (x / plotW) * win.spanMs;
}

export function xFromMs(ms: number, plotW: number, win: TimeWindow): number {
  if (win.spanMs <= 0) return 0;
  return ((ms - win.startMs) / win.spanMs) * plotW;
}

export function projectSample(bar: TimelineBar, plotW: number, win: TimeWindow) {
  const start = xFromMs(bar.startMs, plotW, win);
  const end = xFromMs(barEnd(bar), plotW, win);
  if (end < 0 || start >= plotW || (end === 0 && start < 0)) return null;
  const left = Math.max(0, start);
  const right = Math.min(plotW, Math.max(end, left + 1));
  return { left, right, continuesLeft: start < 0, continuesRight: end > plotW };
}

export function threadHeight(maxDepth: number): number {
  const rows = Math.max(1, maxDepth + 1);
  return THREAD_PAD * 2 + rows * BAR_H + Math.max(0, rows - 1) * BAR_GAP;
}

export function barY(threadTop: number, depth: number): number {
  return threadTop + THREAD_PAD + depth * (BAR_H + BAR_GAP);
}

export function layoutHeight(threads: TimelineThread[]): number {
  if (threads.length === 0) return RULER_H;
  let h = RULER_H;
  threads.forEach((t, i) => {
    h += threadHeight(t.maxDepth);
    if (i < threads.length - 1) h += THREAD_GAP;
  });
  return h;
}

export interface TickMark {
  ms: number;
  label: string;
}

/** 按可见窗口选 1/2/5×10^n 的刻度，对齐 Unity 时间尺。 */
export function timeTicks(win: TimeWindow, plotW: number): TickMark[] {
  if (!(win.spanMs > 0) || !(plotW > 0)) return [];
  const target = 70;
  const raw = win.spanMs / Math.max(1, plotW / target);
  const mag = 10 ** Math.floor(Math.log10(Math.max(raw, 1e-6)));
  let step = mag;
  for (const m of [1, 2, 5, 10]) {
    if (m * mag >= raw * 0.8) {
      step = m * mag;
      break;
    }
  }
  const start = Math.ceil(win.startMs / step) * step;
  const end = windowEnd(win);
  const out: TickMark[] = [];
  for (let t = start; t <= end + step * 1e-6; t += step) {
    out.push({ ms: t, label: formatTick(t, step) });
  }
  return out;
}

function formatTick(ms: number, _step: number): string {
  return `${ms.toFixed(2)}ms`;
}

function layoutNode(
  node: SampleNode,
  lane: CpuThreadSamples,
  depth: number,
  parentPath: string[],
  bars: TimelineBar[],
): void {
  const samplePath = [...parentPath, node.name];
  const path = samplePath.join('/');
  const duration = Math.max(0, node.totalMs);
  const start = lane.offsetMs + (node.startMs ?? 0);
  const id = `${lane.id}:${path}@${start.toFixed(4)}:${bars.length}`;
  if (node.startMs !== undefined) bars.push({
    id,
    path,
    samplePath,
    name: node.name,
    category: node.category,
    color: categoryColor(node.category, node.name),
    startMs: start,
    durationMs: duration,
    selfMs: node.selfMs,
    calls: node.calls,
    callstack: node.callstack,
    depth,
    thread: lane.name,
  });
  for (const child of node.children ?? []) {
    layoutNode(child, lane, node.startMs === undefined ? depth : depth + 1, samplePath, bars);
  }
}

function maxDepthOf(bars: TimelineBar[]): number {
  return bars.reduce((m, b) => Math.max(m, b.depth), 0);
}

export function layoutTimeline(nodes: SampleNode[], frame: FrameData | null, threadSamples?: CpuThreadSamples[]): TimelineLayout {
  const lanes = threadSamples ?? [{ id: 'main', name: 'Main Thread', offsetMs: 0, samples: nodes }];
  const threads: TimelineThread[] = [];
  for (const lane of lanes) {
    const bars: TimelineBar[] = [];
    for (const root of lane.samples) layoutNode(root, lane, 0, [], bars);
    if (bars.length > 0) threads.push({ id: lane.id, name: lane.name, bars, maxDepth: maxDepthOf(bars) });
  }
  let startMs = 0;
  let totalMs = Math.max(frame?.mainThreadMs || 0, frame?.cpuTimeMs || 0);
  for (const th of threads) {
    for (const bar of th.bars) {
      startMs = Math.min(startMs, bar.startMs);
      totalMs = Math.max(totalMs, barEnd(bar));
    }
  }
  return { threads, startMs, totalMs: Math.max(totalMs, MIN_SPAN_MS) };
}

export function layoutTimelineContext(current: TimelineLayout, context: CpuTimelineFrame[]): TimelineLayout {
  if (!context.some((frame) => !frame.selected)) return current;
  const threads = new Map(current.threads.map((thread) => [thread.id, { ...thread, bars: [] as TimelineBar[] }]));
  let { startMs, totalMs } = current;
  for (const frame of context) {
    if (frame.selected) continue;
    const layout = layoutTimeline([], null, frame.threads);
    startMs = Math.min(startMs, frame.offsetMs + layout.startMs);
    totalMs = Math.max(totalMs, frame.offsetMs + Math.max(frame.durationMs, layout.totalMs));
    for (const thread of layout.threads) {
      let lane = threads.get(thread.id);
      if (!lane) {
        lane = { ...thread, bars: [] };
        threads.set(thread.id, lane);
      }
      lane.maxDepth = Math.max(lane.maxDepth, thread.maxDepth);
      for (const bar of thread.bars) {
        lane.bars.push({ ...bar, id: `${frame.frameIndex}:${bar.id}`, ghost: true, startMs: bar.startMs + frame.offsetMs });
      }
    }
  }
  for (const thread of current.threads) {
    const lane = threads.get(thread.id);
    if (lane) lane.bars = lane.bars.concat(thread.bars);
  }
  return { startMs, totalMs, threads: [...threads.values()] };
}

export function hitTest(
  threads: TimelineThread[],
  x: number,
  y: number,
  plotLeft: number,
  plotW: number,
  win: TimeWindow,
): TimelineBar | null {
  const plotX = x - plotLeft;
  if (y < RULER_H || plotW <= 0 || plotX < 0 || plotX >= plotW) return null;
  let top = RULER_H;
  let best: TimelineBar | null = null;
  for (let i = 0; i < threads.length; i++) {
    const th = threads[i];
    const h = threadHeight(th.maxDepth);
    if (y >= top && y < top + h) {
      for (const bar of th.bars) {
        if (bar.ghost || bar.depth > th.maxDepth) continue;
        const by = barY(top, bar.depth);
        if (y >= by && y < by + BAR_H) {
          const bounds = projectSample(bar, plotW, win);
          if (bounds && plotX >= bounds.left && plotX < bounds.right) best = bar;
        }
      }
      return best;
    }
    top += h + THREAD_GAP;
  }
  return null;
}

export function panDeltaFromPixels(dx: number, plotW: number, win: TimeWindow): number {
  if (plotW <= 0) return 0;
  return -(dx / plotW) * win.spanMs;
}
