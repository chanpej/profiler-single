import { describe, expect, it, vi } from 'vitest';
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
    callstack: ['PlayerController.Update()', 'GameLoop.Tick()'],
  }],
}];

function mockPlot(el: HTMLElement, width = 400, height = 180) {
  Object.defineProperty(el, 'clientWidth', { configurable: true, get: () => width });
  Object.defineProperty(el, 'clientHeight', { configurable: true, get: () => height });
  Object.defineProperty(el, 'scrollTop', { configurable: true, writable: true, value: 0 });
  el.getBoundingClientRect = () => ({
    x: 0, y: 0, top: 0, left: 0, right: width, bottom: height, width, height, toJSON: () => ({}),
  });
}

describe('CpuTimeline', () => {
  it('shows operation hints in the bottom bar instead of a plot title tooltip', () => {
    render(<CpuTimeline frame={frame} hierarchy={tree} />);
    const timeline = screen.getByTestId('cpu-timeline');
    const plot = screen.getByTestId('cpu-timeline-plot');
    const footer = screen.getByTestId('cpu-timeline-footer');
    expect(plot).not.toHaveAttribute('title');
    expect(plot).not.toContainElement(footer);
    expect(timeline.lastElementChild).toBe(footer);
    expect(footer).toHaveTextContent('中键拖拽平移 · 滚轮缩放');
    expect(footer).toHaveTextContent('A 适应整帧 · F 聚焦采样');
  });

  it('keeps the tooltip under a stationary pointer when sample data refreshes', () => {
    const { rerender } = render(<CpuTimeline frame={frame} hierarchy={tree} />);
    const plot = screen.getByTestId('cpu-timeline-plot');
    mockPlot(plot);
    fireEvent.pointerMove(plot, { clientX: THREAD_LABEL_W + 50, clientY: RULER_H + 4 + BAR_H + 3 });
    expect(screen.getByRole('tooltip')).toHaveTextContent('BehaviourUpdate3.00ms');
    rerender(<CpuTimeline frame={frame} hierarchy={[{ ...tree[0], children: [
      { ...tree[0].children![0], totalMs: 4 },
    ] }]} />);
    expect(screen.getByRole('tooltip')).toHaveTextContent('BehaviourUpdate4.00ms');
    fireEvent.pointerLeave(plot);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });

  it('switches from duration-first labels to combined labels that fade as their bars narrow', () => {
    const fillText = vi.fn();
    const childAlpha: number[] = [];
    const context = {
      globalAlpha: 1,
      fillText,
      measureText: (text: string) => ({ width: text.length * 5 }),
      setTransform: vi.fn(), clearRect: vi.fn(), fillRect: vi.fn(),
      beginPath: vi.fn(), moveTo: vi.fn(), lineTo: vi.fn(), stroke: vi.fn(),
      save: vi.fn(), rect: vi.fn(), clip: vi.fn(), restore: vi.fn(),
    };
    const getContext = vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context as unknown as CanvasRenderingContext2D);
    try {
      fillText.mockImplementation((label: string) => {
        if (label === 'BehaviourUpdate (3.00ms)') childAlpha.push(context.globalAlpha);
      });
      const { rerender, unmount } = render(<CpuTimeline frame={frame} hierarchy={tree} />);
      mockPlot(screen.getByTestId('cpu-timeline-plot'), THREAD_LABEL_W + 220);
      rerender(<CpuTimeline frame={frame} hierarchy={[...tree]} />);
      const rootCenter = RULER_H + 4 + BAR_H / 2;
      const childCenter = rootCenter + BAR_H + 1;
      expect(fillText).toHaveBeenCalledWith('10.00ms', expect.any(Number), rootCenter);
      expect(fillText).toHaveBeenCalledWith('3.00ms', expect.any(Number), childCenter);
      expect(fillText).not.toHaveBeenCalledWith('BehaviourUpdate', expect.any(Number), childCenter);
      const select = screen.getByLabelText('文字样式', { exact: false });
      expect(select.closest('label')?.nextElementSibling).toBe(screen.getByRole('button', { name: '适应窗口' }));
      expect(select).toHaveValue('split');
      fireEvent.change(select, { target: { value: 'unity' } });
      expect(childAlpha.length).toBeGreaterThan(0);
      const medium = childAlpha[childAlpha.length - 1];
      expect(medium).toBeGreaterThan(0);
      expect(medium).toBeLessThan(1);
      const alphaAtWidth = (width: number) => {
        childAlpha.length = 0;
        mockPlot(screen.getByTestId('cpu-timeline-plot'), THREAD_LABEL_W + width);
        rerender(<CpuTimeline frame={frame} hierarchy={[...tree]} />);
        return childAlpha[childAlpha.length - 1];
      };
      const narrow = alphaAtWidth(100);
      expect(narrow).toBeGreaterThan(0);
      expect(narrow).toBeLessThan(medium);
      expect(alphaAtWidth(900)).toBe(1);
      unmount();
      render(<CpuTimeline frame={frame} hierarchy={tree} />);
      expect(screen.getByLabelText('文字样式', { exact: false })).toHaveValue('unity');
    } finally {
      localStorage.removeItem('upa.cpuTimeline.labelStyle');
      getContext.mockRestore();
    }
  });

  it('pans into either neighbor at the selected frame scale and fits back to the selected frame', () => {
    const threads = [{ id: 'main', name: 'Main Thread', offsetMs: 0, samples: tree }];
    render(<CpuTimeline frame={frame} hierarchy={tree} context={[
      { frameIndex: 2, selected: false, offsetMs: -12, durationMs: 12, threads },
      { frameIndex: 3, selected: true, offsetMs: 0, durationMs: 10, threads },
      { frameIndex: 4, selected: false, offsetMs: 10, durationMs: 20, threads },
    ]} />);
    const plot = screen.getByTestId('cpu-timeline-plot');
    mockPlot(plot);
    const window = screen.getByTestId('cpu-timeline-window');
    expect(window).toHaveAttribute('data-start', '0');
    expect(window).toHaveAttribute('data-span', '10');
    const width = 400 - THREAD_LABEL_W;
    const drag = (startX: number, endX: number) => {
      fireEvent.pointerDown(plot, { clientX: startX, clientY: 40, button: 1, pointerId: 1 });
      fireEvent.pointerMove(plot, { clientX: endX, clientY: 40, pointerId: 1 });
      fireEvent.pointerUp(plot, { clientX: endX, clientY: 40, pointerId: 1 });
    };
    drag(THREAD_LABEL_W, THREAD_LABEL_W + width);
    expect(window).toHaveAttribute('data-start', '-10');
    const clientX = THREAD_LABEL_W + width * 0.2;
    const clientY = RULER_H + 4 + 2;
    fireEvent.pointerDown(plot, { clientX, clientY, button: 0, pointerId: 1 });
    fireEvent.pointerUp(plot, { clientX, clientY, button: 0, pointerId: 1 });
    expect(screen.queryByRole('region', { name: '采样调用栈' })).not.toBeInTheDocument();
    fireEvent.keyDown(plot, { key: 'a' });
    drag(THREAD_LABEL_W + width, THREAD_LABEL_W);
    expect(window).toHaveAttribute('data-start', '10');
    expect(window).toHaveAttribute('data-span', '10');
    fireEvent.keyDown(plot, { key: 'a' });
    expect(window).toHaveAttribute('data-start', '0');
  });

  it('extends the viewport without selecting frames and ignores late pages after release', async () => {
    const seek = vi.fn();
    const rows = (center: number) => Array.from({ length: 7 }, (_, position) => ({
      frameIndex: center + position - 3, offsetMs: (position - 3) * 10, durationMs: 10,
      selected: position === 3, threads: [],
    }));
    let finish!: () => void;
    const load = vi.fn(async (index: number, id: number) => {
      expect(index).toBe(id);
      if (id === 14) await new Promise<void>((resolve) => { finish = resolve; });
      return rows(id);
    });
    const { act, waitFor } = await import('@testing-library/react');
    const { ProfilerDetail } = await import('./ProfilerDetail');
    render(<ProfilerDetail moduleId="cpu" frame={{ ...frame, frameIndex: 20 }} cpuTab="timeline"
      totalFrames={100} timelineSelection={20} onTimelineSeek={seek} getTimeline={load} />);
    const plot = screen.getByTestId('cpu-timeline-plot');
    const view = screen.getByTestId('cpu-timeline-window');
    await waitFor(() => expect(view).toHaveAttribute('data-span', '10'));
    mockPlot(plot, THREAD_LABEL_W + 100);
    const pointer = { pointerId: 1, button: 1, clientY: 40 };
    fireEvent.pointerDown(plot, { ...pointer, clientX: 200 });
    fireEvent.pointerMove(plot, { ...pointer, clientX: -450 });
    await waitFor(() => expect(view).toHaveAttribute('data-start', '65'));
    fireEvent.pointerMove(plot, { ...pointer, clientX: 900 });
    await waitFor(() => expect(load).toHaveBeenLastCalledWith(14, 14));
    fireEvent.pointerMove(plot, { ...pointer, clientX: 150 });
    fireEvent.pointerUp(plot, { ...pointer, clientX: 150 });
    await act(async () => finish());
    expect(view).toHaveAttribute('data-start', '5');
    expect(view).toHaveAttribute('data-span', '10');
    expect(load.mock.calls.map(([index]) => index)).toEqual([20, 23, 26, 17, 14]);
    expect(seek).not.toHaveBeenCalled();
  });

  it('focuses the selected sample with F and fits the frame with A', () => {
    render(<CpuTimeline frame={frame} hierarchy={tree} />);
    const plot = screen.getByTestId('cpu-timeline-plot');
    mockPlot(plot);
    const timelineWindow = screen.getByTestId('cpu-timeline-window');
    const fitted = Number(timelineWindow.getAttribute('data-span'));
    const clientX = THREAD_LABEL_W + xFromMs(2.5, 400 - THREAD_LABEL_W, fitWindow(fitted));
    const clientY = RULER_H + 4 + BAR_H + 2;
    fireEvent.pointerDown(plot, { clientX, clientY, button: 0, pointerId: 1 });
    fireEvent.pointerUp(plot, { clientX, clientY, button: 0, pointerId: 1 });
    fireEvent.keyDown(plot, { key: 'f' });
    expect(screen.getByTestId('cpu-timeline-status')).toHaveTextContent('BehaviourUpdate');
    expect(Number(timelineWindow.getAttribute('data-span'))).toBeLessThan(fitted);
    fireEvent.keyDown(plot, { key: 'a' });
    expect(Number(timelineWindow.getAttribute('data-span'))).toBe(fitted);
  });

  it('toggles the visible sample depth and resizes the time window', () => {
    render(<CpuTimeline frame={frame} hierarchy={tree} />);
    const toggle = screen.getByRole('button', { name: /^Main Thread/ });
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    const [start, end] = screen.getAllByRole('slider');
    fireEvent.change(end, { target: { value: '8' } });
    fireEvent.change(start, { target: { value: '2' } });
    const timelineWindow = screen.getByTestId('cpu-timeline-window');
    expect(timelineWindow).toHaveAttribute('data-start', '2');
    expect(timelineWindow).toHaveAttribute('data-span', '6');
    const rangeWindow = screen.getByTestId('cpu-timeline-range-window');
    Object.defineProperty(rangeWindow.parentElement, 'clientWidth', { value: 100 });
    fireEvent.pointerDown(rangeWindow, { clientX: 35, button: 0, pointerId: 3 });
    fireEvent.pointerMove(rangeWindow, { clientX: 45, pointerId: 3 });
    fireEvent.pointerUp(rangeWindow, { clientX: 45, pointerId: 3 });
    expect(Number(timelineWindow.getAttribute('data-start'))).toBeCloseTo(2 + frame.cpuTimeMs / 10);
    expect(timelineWindow).toHaveAttribute('data-span', '6');
  });
  it('展示 Main Thread / Render Thread 泳道', () => {
    render(<CpuTimeline frame={frame} hierarchy={[]} threadSamples={[
      { id: 'main', name: 'Main Thread', offsetMs: 0, samples: tree },
      { id: 'render', name: 'Render Thread', offsetMs: -1, samples: [{ ...tree[0], name: 'Gfx.ProcessCommands' }] },
    ]} />);
    expect(screen.getByTestId('cpu-timeline')).toBeInTheDocument();
    expect(screen.getByTestId('cpu-timeline-threads')).toHaveTextContent('Main Thread');
    expect(screen.getByTestId('cpu-timeline-threads')).toHaveTextContent('Render Thread');
    expect(screen.getByTestId('cpu-timeline-window')).toHaveAttribute('data-start', '-1');
  });

  it('keeps the clicked sample details after the pointer leaves and clears them on an empty click', () => {
    render(<CpuTimeline frame={frame} hierarchy={tree} />);
    const plot = screen.getByTestId('cpu-timeline-plot');
    mockPlot(plot);
    const win = fitWindow(10);
    const x = THREAD_LABEL_W + xFromMs(2.4, 400 - THREAD_LABEL_W, win);
    const y = RULER_H + 4 + BAR_H + 2;
    fireEvent.pointerDown(plot, { clientX: x, clientY: y, button: 0, pointerId: 1 });
    fireEvent.pointerUp(plot, { clientX: x, clientY: y, button: 0, pointerId: 1 });
    fireEvent.pointerLeave(plot);
    expect(screen.getByTestId('cpu-timeline-pinned-tip')).toHaveTextContent('BehaviourUpdate3.00ms');
    expect(screen.getByTestId('cpu-timeline-status')).toHaveTextContent('BehaviourUpdate');
    expect(screen.getByTestId('cpu-timeline-status')).toHaveTextContent('3.00ms');
    expect(screen.getByRole('region', { name: '采样调用栈' })).toHaveTextContent('Main Thread › BehaviourUpdate');
    expect(screen.getByRole('region', { name: '采样调用栈' })).toHaveTextContent('PlayerController.Update()');
    expect(screen.getByTestId('cpu-timeline').lastElementChild).toBe(screen.getByTestId('cpu-timeline-footer'));
    fireEvent.pointerDown(plot, { clientX: x, clientY: RULER_H - 1, button: 0, pointerId: 1 });
    fireEvent.pointerUp(plot, { clientX: x, clientY: RULER_H - 1, button: 0, pointerId: 1 });
    expect(screen.queryByTestId('cpu-timeline-pinned-tip')).not.toBeInTheDocument();
    expect(screen.queryByRole('region', { name: '采样调用栈' })).not.toBeInTheDocument();
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

  it('preserves zoom and pan on sample refresh and frame changes', () => {
    const { rerender } = render(<CpuTimeline frame={frame} hierarchy={tree} />);
    const plot = screen.getByTestId('cpu-timeline-plot');
    const timelineWindow = screen.getByTestId('cpu-timeline-window');
    mockPlot(plot);
    const fitted = timelineWindow.getAttribute('data-span');
    fireEvent.wheel(plot, { clientX: THREAD_LABEL_W + 80, clientY: 40, deltaY: -120 });
    const zoomed = timelineWindow.getAttribute('data-span');
    const start = timelineWindow.getAttribute('data-start');
    expect(Number(zoomed)).toBeLessThan(Number(fitted));
    rerender(<CpuTimeline frame={{ ...frame }} hierarchy={[...tree]} />);
    expect(timelineWindow).toHaveAttribute('data-span', zoomed);
    expect(timelineWindow).toHaveAttribute('data-start', start);
    rerender(<CpuTimeline frame={{ ...frame, frameIndex: frame.frameIndex + 1 }} hierarchy={tree} />);
    expect(timelineWindow).toHaveAttribute('data-span', zoomed);
    expect(timelineWindow).toHaveAttribute('data-start', start);
  });

  it.each([{}, { altKey: true }, { metaKey: true }])('does not pan or select on a primary drag with modifiers %j', (modifiers) => {
    render(<CpuTimeline frame={frame} hierarchy={tree} />);
    const plot = screen.getByTestId('cpu-timeline-plot');
    const timelineWindow = screen.getByTestId('cpu-timeline-window');
    mockPlot(plot);
    fireEvent.wheel(plot, { clientX: THREAD_LABEL_W + 80, clientY: 40, deltaY: -120 });
    const start = timelineWindow.getAttribute('data-start');
    fireEvent.pointerDown(plot, { clientX: 200, clientY: 40, button: 0, pointerId: 2, ...modifiers });
    fireEvent.pointerMove(plot, { clientX: 150, clientY: 28, pointerId: 2, ...modifiers });
    fireEvent.pointerUp(plot, { clientX: 150, clientY: 28, button: 0, pointerId: 2, ...modifiers });
    expect(timelineWindow).toHaveAttribute('data-start', start);
    expect(timelineWindow).toHaveAttribute('data-selected', '');
    expect(plot.scrollTop).toBe(0);
  });

  it('pans horizontally and scrolls vertically with the middle button', () => {
    render(<CpuTimeline frame={frame} hierarchy={tree} />);
    const plot = screen.getByTestId('cpu-timeline-plot');
    mockPlot(plot);
    fireEvent.wheel(plot, { clientX: THREAD_LABEL_W + 80, clientY: 40, deltaY: -120 });
    const start0 = Number(screen.getByTestId('cpu-timeline-window').getAttribute('data-start'));
    fireEvent.pointerDown(plot, { clientX: 200, clientY: 40, button: 1, pointerId: 2 });
    expect(plot).toHaveClass('is-panning');
    fireEvent.pointerMove(plot, { clientX: 140, clientY: 28, pointerId: 2 });
    fireEvent.pointerUp(plot, { clientX: 140, clientY: 28, button: 1, pointerId: 2 });
    const start1 = Number(screen.getByTestId('cpu-timeline-window').getAttribute('data-start'));
    expect(start1).toBeGreaterThan(start0);
    expect(plot.scrollTop).toBe(12);
    expect(plot).not.toHaveClass('is-panning');
  });
});

it('keeps panned lanes visible while activating a neighbor with a delayed backend response', async () => {
  const { waitFor, act } = await import('@testing-library/react');
  const { ProfilerDetail } = await import('./ProfilerDetail');
  const threads = [{ id: 'worker', name: 'Loading Worker', offsetMs: 0, samples: tree }];
  const rows = (center: number) => [20, 21, 22].map(id => ({ frameIndex: id, selected: id === center,
    offsetMs: (id - center) * 10, durationMs: 10, threads }));
  let finish!: () => void;
  const load = async (_: number, id: number) => {
    if (id === 21) await new Promise<void>(resolve => { finish = resolve; });
    return rows(id);
  };
  const seek = vi.fn();
  const detail = (id: number) => <ProfilerDetail moduleId="cpu" frame={{ ...frame, frameIndex: id }} cpuTab="timeline"
    totalFrames={23} timelineSelection={id} onTimelineSeek={seek} getTimeline={load} />;
  const view = render(detail(20));
  const lanes = screen.getByTestId('cpu-timeline-threads');
  await waitFor(() => expect(lanes).toHaveTextContent('Loading Worker'));
  const plot = screen.getByTestId('cpu-timeline-plot');
  mockPlot(plot, THREAD_LABEL_W + 100);
  const pan = { pointerId: 1, button: 1, clientY: 40 };
  fireEvent.pointerDown(plot, { ...pan, clientX: 230 });
  fireEvent.pointerMove(plot, { ...pan, clientX: 130 });
  fireEvent.pointerUp(plot, { ...pan, clientX: 130 });
  const click = { pointerId: 2, button: 0, clientX: THREAD_LABEL_W + 30, clientY: 40 };
  fireEvent.pointerDown(plot, click);
  fireEvent.pointerUp(plot, click);
  expect(seek).toHaveBeenCalledWith(21);
  view.rerender(detail(21));
  expect(lanes).toHaveTextContent('Loading Worker');
  const viewport = screen.getByTestId('cpu-timeline-window');
  expect(viewport).toHaveAttribute('data-start','0');
  expect(viewport).toHaveAttribute('data-span','10');
  await act(async () => finish());
  expect(lanes).toHaveTextContent('Loading Worker');
  expect(viewport).toHaveAttribute('data-start','0');
});
