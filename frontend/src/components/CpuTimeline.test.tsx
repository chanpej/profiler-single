import { describe, expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { CpuTimeline } from './CpuTimeline';
import { emptyFrame } from '../lib/frame';
import type { SampleNode } from '../types';
import { THREAD_LABEL_W, xFromMs, fitWindow, RULER_H, BAR_H } from '../lib/cpuTimeline';

const frame = emptyFrame({
  frameIndex: 3,
  cpuTimeMs: 16.4,
  cpuRenderingMs: 5,
  cpuScriptsMs: 4,
  mainThreadMs: 14,
  renderMs: 6,
});

const tree: SampleNode[] = [{
  name: 'Main Thread',
  category: 'Internal',
  totalMs: 10,
  selfMs: 1,
  calls: 1,
  gcAllocKB: 0,
  startMs: 0,
  children: [{
    name: 'BehaviourUpdate',
    category: 'Scripts',
    totalMs: 3,
    selfMs: 3,
    calls: 1,
    gcAllocKB: 0,
    startMs: 2,
  }],
}];

function mockPlot(el: HTMLElement, width = 400, height = 180) {
  Object.defineProperty(el, 'clientWidth', { configurable: true, get: () => width });
  Object.defineProperty(el, 'clientHeight', { configurable: true, get: () => height });
  Object.defineProperty(el, 'scrollTop', { configurable: true, get: () => 0 });
  el.getBoundingClientRect = () => ({
    x: 0, y: 0, top: 0, left: 0, right: width, bottom: height, width, height, toJSON: () => ({}),
  });
}

describe('CpuTimeline', () => {
  it('展示 Main Thread / Render Thread 泳道', () => {
    render(<CpuTimeline frame={frame} hierarchy={[]} />);
    expect(screen.getByTestId('cpu-timeline')).toBeInTheDocument();
    expect(screen.getByTestId('cpu-timeline-threads')).toHaveTextContent('Main Thread');
    expect(screen.getByTestId('cpu-timeline-threads')).toHaveTextContent('Render Thread');
  });

  it('单击采样条选中，状态栏显示名称与耗时', () => {
    render(<CpuTimeline frame={frame} hierarchy={tree} />);
    const plot = screen.getByTestId('cpu-timeline-plot');
    mockPlot(plot);
    const win = fitWindow(10);
    const x = THREAD_LABEL_W + xFromMs(2.4, 400 - THREAD_LABEL_W, win);
    const y = RULER_H + 4 + BAR_H + 2;
    fireEvent.pointerDown(plot, { clientX: x, clientY: y, button: 0, pointerId: 1 });
    fireEvent.pointerUp(plot, { clientX: x, clientY: y, button: 0, pointerId: 1 });
    expect(screen.getByTestId('cpu-timeline-status')).toHaveTextContent('BehaviourUpdate');
    expect(screen.getByTestId('cpu-timeline-status')).toHaveTextContent('3.00ms');
  });

  it('滚轮缩小可见窗口，适应按钮恢复整帧', () => {
    render(<CpuTimeline frame={frame} hierarchy={tree} />);
    const plot = screen.getByTestId('cpu-timeline-plot');
    mockPlot(plot);
    const before = Number(screen.getByTestId('cpu-timeline-window').getAttribute('data-span'));
    fireEvent.wheel(plot, { clientX: THREAD_LABEL_W + 80, clientY: 40, deltaY: -80 });
    const zoomed = Number(screen.getByTestId('cpu-timeline-window').getAttribute('data-span'));
    expect(zoomed).toBeLessThan(before);
    fireEvent.click(screen.getByRole('button', { name: '适应窗口' }));
    const fitted = Number(screen.getByTestId('cpu-timeline-window').getAttribute('data-span'));
    expect(fitted).toBeGreaterThanOrEqual(before - 0.01);
  });

  it('拖拽平移改变窗口起点', () => {
    render(<CpuTimeline frame={frame} hierarchy={tree} />);
    const plot = screen.getByTestId('cpu-timeline-plot');
    mockPlot(plot);
    fireEvent.wheel(plot, { clientX: THREAD_LABEL_W + 80, clientY: 40, deltaY: -120 });
    const start0 = Number(screen.getByTestId('cpu-timeline-window').getAttribute('data-start'));
    fireEvent.pointerDown(plot, { clientX: 200, clientY: 40, button: 0, pointerId: 2 });
    fireEvent.pointerMove(plot, { clientX: 140, clientY: 40, pointerId: 2 });
    fireEvent.pointerUp(plot, { clientX: 140, clientY: 40, pointerId: 2 });
    const start1 = Number(screen.getByTestId('cpu-timeline-window').getAttribute('data-start'));
    expect(start1).toBeGreaterThan(start0);
  });
});
