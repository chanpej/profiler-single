import type { FrameData, SampleNode } from '../types';
import { categoryColor } from './hierarchy';
import { moduleById, moduleBreakdown } from './profiler';

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
  name: string;
  category: string;
  color: string;
  startMs: number;
  durationMs: number;
  depth: number;
  thread: string;
}

export interface TimelineThread {
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
  totalMs: number;
}

export function barEnd(bar: TimelineBar): number {
  return bar.startMs + bar.durationMs;
}

export function windowEnd(win: TimeWindow): number {
  return win.startMs + win.spanMs;
}

export function fitWindow(totalMs: number): TimeWindow {
  const span = Math.max(totalMs, MIN_SPAN_MS);
  return { startMs: 0, spanMs: span };
}

export function clampWindow(win: TimeWindow, totalMs: number): TimeWindow {
  const maxSpan = Math.max(totalMs, MIN_SPAN_MS);
  const spanMs = Math.min(maxSpan, Math.max(MIN_SPAN_MS, win.spanMs));
  const maxStart = Math.max(0, maxSpan - spanMs);
  const startMs = Math.min(maxStart, Math.max(0, win.startMs));
  return { startMs, spanMs };
}

/**
 * 围绕 anchorMs 缩放。factor < 1 放大（更短窗口），> 1 缩小。
 */
export function zoomWindow(win: TimeWindow, totalMs: number, factor: number, anchorMs: number): TimeWindow {
  const spanMs = win.spanMs * factor;
  const ratio = win.spanMs <= 0 ? 0 : (anchorMs - win.startMs) / win.spanMs;
  const startMs = anchorMs - ratio * spanMs;
  return clampWindow({ startMs, spanMs }, totalMs);
}

export function panWindow(win: TimeWindow, totalMs: number, deltaMs: number): TimeWindow {
  return clampWindow({ ...win, startMs: win.startMs + deltaMs }, totalMs);
}

/** 双击采样：把该条放到视野中央并略留边。 */
export function zoomToBar(bar: TimelineBar, totalMs: number): TimeWindow {
  const pad = Math.max(bar.durationMs * 0.2, MIN_SPAN_MS * 0.5);
  const spanMs = Math.max(bar.durationMs + pad * 2, MIN_SPAN_MS);
  return clampWindow({ startMs: bar.startMs - pad, spanMs }, totalMs);
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

function hasTimedSamples(nodes: SampleNode[]): boolean {
  const walk = (list: SampleNode[]): boolean => list.some((n) => {
    if ((n.startMs ?? 0) > 0) return true;
    return n.children ? walk(n.children) : false;
  });
  return walk(nodes);
}

function layoutNode(
  node: SampleNode,
  cursor: number,
  depth: number,
  thread: string,
  prefix: string,
  timed: boolean,
  bars: TimelineBar[],
): number {
  const path = prefix ? `${prefix}/${node.name}` : node.name;
  const duration = Math.max(0, node.totalMs);
  const start = timed ? (node.startMs ?? cursor) : cursor;
  const id = `${thread}:${path}@${start.toFixed(4)}:${bars.length}`;
  bars.push({
    id,
    path,
    name: node.name,
    category: node.category,
    color: categoryColor(node.category),
    startMs: start,
    durationMs: duration,
    depth,
    thread,
  });
  let childCursor = start;
  for (const child of node.children ?? []) {
    childCursor = layoutNode(child, childCursor, depth + 1, thread, path, timed, bars);
  }
  return timed ? start + duration : Math.max(start + duration, childCursor);
}

function maxDepthOf(bars: TimelineBar[]): number {
  return bars.reduce((m, b) => Math.max(m, b.depth), 0);
}

function threadFromRootName(name: string): string | null {
  const n = name.trim();
  if (/thread/i.test(n) || /job/i.test(n) || /worker/i.test(n)) return n;
  return null;
}

/**
 * 把采样树铺成 Unity Timeline 式线程泳道。
 * 有 startMs 时按真实时序；否则在父节点内从左到右紧排子节点。
 */
export function layoutTimeline(nodes: SampleNode[], frame: FrameData | null): TimelineLayout {
  const timed = hasTimedSamples(nodes);
  const byThread = new Map<string, TimelineBar[]>();
  const order: string[] = [];
  const ensure = (name: string) => {
    if (!byThread.has(name)) {
      byThread.set(name, []);
      order.push(name);
    }
    return byThread.get(name)!;
  };

  if (nodes.length > 0) {
    let fallbackCursor = 0;
    for (const root of nodes) {
      const thread = threadFromRootName(root.name) ?? 'Main Thread';
      const bars = ensure(thread);
      fallbackCursor = layoutNode(root, timed ? (root.startMs ?? 0) : fallbackCursor, 0, thread, '', timed, bars);
    }
    if (frame && frame.renderMs > 0 && !byThread.has('Render Thread')) {
      ensure('Render Thread').push({
        id: 'render',
        path: 'Render Thread',
        name: 'Render',
        category: 'Render',
        color: categoryColor('Render'),
        startMs: 0,
        durationMs: frame.renderMs,
        depth: 0,
        thread: 'Render Thread',
      });
    }
  } else if (frame) {
    const cats = moduleBreakdown(moduleById('cpu'), frame);
    const mainBars = ensure('Main Thread');
    let t = 0;
    for (const c of cats) {
      if (!(c.value > 0)) continue;
      mainBars.push({
        id: `cpu:${c.id}`,
        path: c.label,
        name: c.label,
        category: c.id,
        color: c.color,
        startMs: t,
        durationMs: c.value,
        depth: 0,
        thread: 'Main Thread',
      });
      t += c.value;
    }
    if (frame.renderMs > 0) {
      ensure('Render Thread').push({
        id: 'render',
        path: 'Render Thread',
        name: 'Render',
        category: 'Render',
        color: categoryColor('Render'),
        startMs: 0,
        durationMs: frame.renderMs,
        depth: 0,
        thread: 'Render Thread',
      });
    }
  }

  const threads: TimelineThread[] = order.map((name) => {
    const bars = byThread.get(name) ?? [];
    return { name, bars, maxDepth: maxDepthOf(bars) };
  });

  let totalMs = 0;
  for (const th of threads) {
    for (const b of th.bars) totalMs = Math.max(totalMs, barEnd(b));
  }
  if (frame) {
    totalMs = Math.max(totalMs, frame.mainThreadMs || 0, frame.cpuTimeMs || 0, frame.renderMs || 0);
  }
  return { threads, totalMs: Math.max(totalMs, MIN_SPAN_MS) };
}

export function hitTest(
  threads: TimelineThread[],
  x: number,
  y: number,
  plotLeft: number,
  plotW: number,
  win: TimeWindow,
): TimelineBar | null {
  if (y < RULER_H || plotW <= 0) return null;
  const t = msFromX(x - plotLeft, plotW, win);
  let top = RULER_H;
  let best: TimelineBar | null = null;
  for (let i = 0; i < threads.length; i++) {
    const th = threads[i];
    const h = threadHeight(th.maxDepth);
    if (y >= top && y < top + h) {
      for (const bar of th.bars) {
        if (t < bar.startMs || t > barEnd(bar)) continue;
        const by = barY(top, bar.depth);
        if (y >= by && y < by + BAR_H) {
          if (!best || bar.depth >= best.depth) best = bar;
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
