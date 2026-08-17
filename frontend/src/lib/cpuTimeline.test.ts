import { describe, expect, it } from 'vitest';
import type { SampleNode } from '../types';
import { emptyFrame } from './frame';
import {
  BAR_H,
  MIN_SPAN_MS,
  RULER_H,
  THREAD_LABEL_W,
  clampWindow,
  fitWindow,
  hitTest,
  layoutHeight,
  layoutTimeline,
  msFromX,
  panDeltaFromPixels,
  panWindow,
  timeTicks,
  wheelZoomFactor,
  xFromMs,
  zoomToBar,
  zoomWindow,
} from './cpuTimeline';

const tree: SampleNode[] = [
  {
    name: 'Main Thread',
    category: 'Internal',
    totalMs: 10,
    selfMs: 1,
    calls: 1,
    gcAllocKB: 0,
    startMs: 0,
    children: [
      {
        name: 'BehaviourUpdate',
        category: 'Scripts',
        totalMs: 3,
        selfMs: 3,
        calls: 1,
        gcAllocKB: 0,
        startMs: 2,
      },
      {
        name: 'BehaviourUpdate',
        category: 'Scripts',
        totalMs: 2,
        selfMs: 2,
        calls: 1,
        gcAllocKB: 0,
        startMs: 6,
      },
    ],
  },
];

describe('layoutTimeline', () => {
  it('按 startMs 把同名采样摆成两条独立时间条', () => {
    const { threads, totalMs } = layoutTimeline(tree, emptyFrame({ mainThreadMs: 10 }));
    expect(threads).toHaveLength(1);
    expect(threads[0].name).toBe('Main Thread');
    const names = threads[0].bars.map((b) => `${b.name}@${b.startMs}`);
    expect(names).toEqual(['Main Thread@0', 'BehaviourUpdate@2', 'BehaviourUpdate@6']);
    expect(threads[0].bars[1].durationMs).toBe(3);
    expect(threads[0].bars[2].durationMs).toBe(2);
    expect(threads[0].maxDepth).toBe(1);
    expect(totalMs).toBeGreaterThanOrEqual(10);
    const withRender = layoutTimeline(tree, emptyFrame({ mainThreadMs: 10, renderMs: 6 }));
    expect(withRender.threads.map((t) => t.name)).toEqual(['Main Thread', 'Render Thread']);
  });

  it('没有 startMs 时在父节点内从左到右紧排子节点', () => {
    const packed: SampleNode[] = [{
      name: 'Main Thread',
      category: 'Internal',
      totalMs: 10,
      selfMs: 1,
      calls: 1,
      gcAllocKB: 0,
      children: [
        { name: 'A', category: 'Scripts', totalMs: 4, selfMs: 4, calls: 1, gcAllocKB: 0 },
        { name: 'B', category: 'Scripts', totalMs: 3, selfMs: 3, calls: 1, gcAllocKB: 0 },
      ],
    }];
    const { threads } = layoutTimeline(packed, null);
    expect(threads[0].bars.map((b) => [b.name, b.startMs])).toEqual([
      ['Main Thread', 0],
      ['A', 0],
      ['B', 4],
    ]);
  });

  it('无采样树时用 CPU 分类色条 + Render Thread', () => {
    const frame = emptyFrame({ cpuTimeMs: 16, cpuRenderingMs: 5, cpuScriptsMs: 4, renderMs: 6, mainThreadMs: 14 });
    const { threads } = layoutTimeline([], frame);
    expect(threads.map((t) => t.name)).toEqual(['Main Thread', 'Render Thread']);
    expect(threads[0].bars[0].name).toBe('Rendering');
    expect(threads[0].bars[0].durationMs).toBe(5);
    expect(threads[1].bars[0].durationMs).toBe(6);
  });
});

describe('TimeWindow', () => {
  it('缩放围绕锚点，平移不越出帧范围', () => {
    const fit = fitWindow(10);
    expect(fit).toEqual({ startMs: 0, spanMs: 10 });
    const zoomed = zoomWindow(fit, 10, 0.5, 4);
    expect(zoomed.spanMs).toBe(5);
    expect(zoomed.startMs).toBeCloseTo(2);
    const panned = panWindow(zoomed, 10, 10);
    expect(panned.startMs + panned.spanMs).toBeCloseTo(10);
    expect(clampWindow({ startMs: -4, spanMs: 100 }, 10).spanMs).toBe(10);
    expect(clampWindow({ startMs: 0, spanMs: 0.01 }, 10).spanMs).toBe(MIN_SPAN_MS);
  });

  it('滚轮向上放大，双击采样对准该条', () => {
    expect(wheelZoomFactor(-1)).toBeLessThan(1);
    expect(wheelZoomFactor(1)).toBeGreaterThan(1);
    const bar = layoutTimeline(tree, null).threads[0].bars[1];
    const win = zoomToBar(bar, 10);
    expect(win.startMs).toBeLessThanOrEqual(bar.startMs);
    expect(win.startMs + win.spanMs).toBeGreaterThanOrEqual(bar.startMs + bar.durationMs);
  });

  it('像素与时间互转，拖拽方向与 Unity 一致（向右拖看更早）', () => {
    const win = { startMs: 0, spanMs: 10 };
    expect(msFromX(100, 200, win)).toBe(5);
    expect(xFromMs(5, 200, win)).toBe(100);
    expect(panDeltaFromPixels(40, 200, win)).toBeCloseTo(-2);
  });
});

describe('hitTest / ticks', () => {
  it('点中嵌套条时选中更深层采样', () => {
    const { threads } = layoutTimeline(tree, null);
    const plotW = 200;
    const win = fitWindow(10);
    const x = THREAD_LABEL_W + xFromMs(2.5, plotW, win);
    const y = RULER_H + 4 + BAR_H + 1 + 2;
    const hit = hitTest(threads, x, y, THREAD_LABEL_W, plotW, win);
    expect(hit?.name).toBe('BehaviourUpdate');
    expect(hit?.startMs).toBe(2);
  });

  it('刻度落在可见窗口内', () => {
    const ticks = timeTicks({ startMs: 0, spanMs: 10 }, 400);
    expect(ticks.length).toBeGreaterThan(1);
    expect(ticks[0].ms).toBeGreaterThanOrEqual(0);
    expect(ticks[ticks.length - 1].ms).toBeLessThanOrEqual(10.001);
    expect(ticks[0].label).toMatch(/ms$/);
  });

  it('布局高度随嵌套层数增加', () => {
    const { threads } = layoutTimeline(tree, null);
    expect(layoutHeight(threads)).toBeGreaterThan(RULER_H + BAR_H);
  });
});
