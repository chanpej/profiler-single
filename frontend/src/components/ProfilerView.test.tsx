import { describe, expect, it, vi, afterEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfilerView } from './ProfilerView';
import { emptyFrame } from '../lib/frame';
import { THREAD_LABEL_W } from '../lib/cpuTimeline';
import type { CpuTimelineFrame, FrameData } from '../types';
import type { ComponentProps } from 'react';

function frameAt(i: number): FrameData {
  return emptyFrame({
    frameIndex: 100 + i,
    timestampMs: 1755072000000 + i * 16,
    fps: 60,
    cpuTimeMs: 16.6,
    gpuTimeMs: 12,
    mainThreadMs: 14,
    renderMs: 6,
    cpuRenderingMs: 5,
    cpuScriptsMs: 4,
    cpuPhysicsMs: 2,
    usedMemMB: 512,
    gcUsedMB: 180,
    gfxUsedMB: 160,
    drawCalls: 300,
    triangles: 900000,
    playingSources: 8,
    activeRigidbodies: 40,
    uiBatches: 22,
  });
}

const frames = [frameAt(0), frameAt(1), frameAt(2)];

const noop = () => {};

function renderView(over: Partial<ComponentProps<typeof ProfilerView>> = {}) {
  return render(
    <ProfilerView
      frames={frames}
      recording
      connected
      onClear={noop}
      onToggleRecord={noop}
      onPause={noop}

      onImport={noop}
      {...over}
    />,
  );
}

describe('ProfilerView', () => {
  it('默认展示 CPU/GPU/Rendering/Memory 模块图', () => {
    renderView();
    expect(screen.getByTestId('profiler-chart-cpu')).toBeInTheDocument();
    expect(screen.getByTestId('profiler-chart-memory')).toBeInTheDocument();
    expect(screen.queryByTestId('profiler-chart-audio')).not.toBeInTheDocument();
    for (const id of ['cpu', 'gpu', 'rendering', 'memory']) {
      expect(screen.getByTestId(`profiler-chart-${id}`)).toHaveAttribute('data-y-axis-width', '42');
    }
  });

  it('Memory 图使用 Unity Editor 的内存与对象指标', () => {
    renderView();
    const chart = screen.getByTestId('profiler-chart-memory');
    expect(chart).toHaveAttribute('data-memory', 'true');
    expect(chart).toHaveClass('profiler-chart--memory');
    expect(chart).toHaveTextContent('Total Memory');
    expect(chart).toHaveTextContent('Texture Memory');
    expect(chart).toHaveTextContent('Material Count');
    expect(chart).toHaveTextContent('Audio Used Memory');
    expect(chart).toHaveTextContent('GC Allocated In Frame');
  });

  it('.data 的仅 Audio Memory 帧仍显示有效曲线和当前值', () => {
    const dataFrame = emptyFrame({ memoryCounterMask: 1 << 6, audioUsedMB: 3.5 });
    renderView({ frames: [dataFrame] });
    const chart = screen.getByTestId('profiler-chart-memory');
    expect(chart).toHaveTextContent('Audio Used Memory');
    expect(chart).toHaveTextContent('3.5 MB');
    expect(chart).toHaveTextContent('—');
  });

  it('可开关 Profiler 模块', async () => {
    renderView();
    await userEvent.click(screen.getByLabelText('Audio'));
    expect(screen.getByTestId('profiler-chart-audio')).toBeInTheDocument();
    await userEvent.click(screen.getByLabelText('CPU Usage'));
    expect(screen.queryByTestId('profiler-chart-cpu')).not.toBeInTheDocument();
  });

  it('所有可见 Module 轨道都参与图表区高度分配', async () => {
    renderView();
    const cpu = screen.getByTestId('profiler-chart-cpu');
    expect(cpu).toHaveClass('profiler-chart--fill');
    expect(cpu).toHaveClass('profiler-chart--cpu');
    expect(screen.getByTestId('profiler-chart-gpu')).toHaveClass('profiler-chart--fill');

    await userEvent.click(screen.getByLabelText('GPU Usage'));
    await userEvent.click(screen.getByLabelText('Rendering'));
    await userEvent.click(screen.getByLabelText('Memory'));
    expect(cpu).toHaveClass('profiler-chart--fill');
  });

  it('帧前进后退与 Current', async () => {
    renderView();
    expect(screen.getByLabelText('当前帧')).toHaveValue('3');
    await userEvent.click(screen.getByRole('button', { name: '上一帧' }));
    expect(screen.getByLabelText('当前帧')).toHaveValue('2');
    await userEvent.click(screen.getByRole('button', { name: '最新帧' }));
    expect(screen.getByLabelText('当前帧')).toHaveValue('');
    expect(screen.getByText('选择一帧以查看详情')).toBeInTheDocument();
  });

  it('点击 Memory 模块后详情切换为 Simple 内存表', async () => {
    renderView();
    await userEvent.click(screen.getByLabelText('Memory'));
    expect(screen.getByTestId('memory-simple')).toBeInTheDocument();
    expect(screen.getByText('Graphics & Graphics Driver')).toBeInTheDocument();
  });

  it('Clear 按钮触发回调', async () => {
    const onClear = vi.fn();
    renderView({ recording: false, connected: false, onClear });
    await userEvent.click(screen.getByRole('button', { name: '清空数据' }));
    expect(onClear).toHaveBeenCalled();
  });

  it('opens Left Heavy from a reverse Shift drag and shows weighted averages', async () => {
    const hot = { name: 'Hot.Update', category: 'Scripts', calls: 4, totalMs: 10, selfMs: 6 };
    const frequent = { name: 'FrequentlyCalled', category: 'Scripts', calls: 8, totalMs: 4, selfMs: 4 };
    const getHeavyProfile = vi.fn(async () => ({
      firstFrame: 100, lastFrame: 102, frameCount: 3,
      threads: [{ id: 'main', name: 'Main Thread', roots: [hot, frequent], functions: [hot, frequent] }],
    }));
    renderView({ getHeavyProfile, recording: false });
    const chart = screen.getByTestId('profiler-chart-cpu').querySelector('.profiler-chart-canvas')!;
    vi.spyOn(chart, 'getBoundingClientRect').mockReturnValue({ left: 0, width: 342 } as DOMRect);
    fireEvent.pointerDown(chart, { button: 0, pointerId: 4, clientX: 290, shiftKey: true });
    fireEvent.pointerMove(chart, { pointerId: 4, clientX: 80, shiftKey: true });
    fireEvent.pointerUp(chart, { pointerId: 4, clientX: 80, shiftKey: false });
    const table = await screen.findByRole('table', { name: '函数汇总' });
    expect(getHeavyProfile).toHaveBeenCalledWith(0, 2);
    expect(screen.getByRole('tab', { name: 'Left Heavy' })).toHaveAttribute('aria-selected', 'true');
    expect(within(table).getAllByRole('row')[1]).toHaveTextContent('Hot.Update42.50ms10.00ms6.00ms');
    await userEvent.click(within(table).getByRole('button', { name: '调用次数' }));
    expect(within(table).getAllByRole('row')[1]).toHaveTextContent('FrequentlyCalled');
    fireEvent.change(screen.getByRole('spinbutton', { name: '起始帧序号' }), { target: { value: '2' } });
    await userEvent.click(screen.getByRole('button', { name: '分析范围' }));
    await waitFor(() => expect(getHeavyProfile).toHaveBeenLastCalledWith(1, 2));
    expect(chart).toHaveAttribute('data-frame-range', '1:2');

    await userEvent.click(screen.getByRole('tab', { name: 'Timeline' }));
    expect(screen.getByRole('tab', { name: 'Timeline' })).toHaveAttribute('aria-selected', 'true');
    expect(chart).toHaveAttribute('data-frame-range', '0:0');

    await userEvent.click(screen.getByRole('tab', { name: 'Left Heavy' }));
    expect(chart).toHaveAttribute('data-frame-range', '1:2');
    expect(getHeavyProfile).toHaveBeenLastCalledWith(1, 2);
  });

  it('CPU Timeline 加载选中帧的线程采样，切帧后替换泳道', async () => {
    const sample = { name: 'PlayerLoop', category: 'Scripts', startMs: 0, totalMs: 8, selfMs: 8, calls: 1, gcAllocKB: 0 };
    const getTimeline = vi.fn(async (index: number) => [{
      frameIndex: frames[index].frameIndex, selected: true, offsetMs: 0, durationMs: 16,
      threads: [{ id: String(index), name: index === 2 ? 'Render Thread' : 'Main Thread', offsetMs: 0, samples: [sample] }],
    }]);
    renderView({ getTimeline, recording: false });
    await userEvent.click(screen.getByRole('tab', { name: 'Timeline' }));
    expect(screen.getByTestId('cpu-timeline')).toBeInTheDocument();
    expect(await screen.findByText('Render Thread')).toBeInTheDocument();
    expect(getTimeline).toHaveBeenCalledWith(2, frames[2].frameIndex);
    await userEvent.click(screen.getByRole('button', { name: '上一帧' }));
    expect(await screen.findByText('Main Thread')).toBeInTheDocument();
    expect(screen.queryByText('Render Thread')).not.toBeInTheDocument();
    expect(getTimeline).toHaveBeenLastCalledWith(1, frames[1].frameIndex);
  });

  it('clicks a dimmed frame and retains the viewport through asynchronous frame changes', async () => {
    const getFrame = vi.fn(async (index: number) => frames[index]);
    const getTimeline = vi.fn(async (index: number): Promise<CpuTimelineFrame[]> => frames.map((frame, position) => ({
      frameIndex: frame.frameIndex, selected: position === index, offsetMs: (position - index) * 16, durationMs: 16,
      threads: [{ id: 'main', name: 'Main Thread', offsetMs: 0, samples: [
        { name: 'PlayerLoop', category: 'Scripts', startMs: 0, totalMs: 8, selfMs: 8, calls: 1, gcAllocKB: 0 },
      ] }],
    })));
    renderView({ frames: undefined, totalCount: frames.length, getFrame, getTimeline, recording: false });
    await userEvent.click(screen.getByRole('tab', { name: 'Timeline' }));
    await screen.findByText('Main Thread');
    const plot = screen.getByTestId('cpu-timeline-plot');
    const view = screen.getByTestId('cpu-timeline-window');
    const width = 320;
    const cpuChart = screen.getByTestId('profiler-chart-cpu').querySelector('.profiler-chart-canvas');
    expect(cpuChart).toHaveAttribute('data-frame-range', '2:2');
    Object.defineProperty(plot, 'clientWidth', { value: THREAD_LABEL_W + width });
    fireEvent.wheel(plot, { clientX: THREAD_LABEL_W + width / 2, deltaY: 100 });
    expect(view).toHaveAttribute('data-span', '20');
    expect(cpuChart).toHaveAttribute('data-frame-range', '1:2');
    const start = Number(view.getAttribute('data-start'));
    const click = { clientX: THREAD_LABEL_W + width / 20, clientY: 55, button: 0, pointerId: 1 };
    fireEvent.pointerDown(plot, click);
    fireEvent.pointerUp(plot, click);
    await waitFor(() => expect(getTimeline).toHaveBeenLastCalledWith(1, frames[1].frameIndex));
    expect(screen.getByTestId('cpu-timeline-plot')).toBe(plot);
    expect(view).toHaveAttribute('data-start', String(start + 16));
    expect(view).toHaveAttribute('data-span', '20');
    await userEvent.click(screen.getByRole('button', { name: '下一帧' }));
    await waitFor(() => expect(getTimeline).toHaveBeenLastCalledWith(2, frames[2].frameIndex));
    expect(view).toHaveAttribute('data-span', '20');
  });

  it('Record / Pause / Load 触发回调', async () => {
    const onToggleRecord = vi.fn();
    const onPause = vi.fn();
    const onImport = vi.fn();
    renderView({ onToggleRecord, onPause, onImport });
    await userEvent.click(screen.getByRole('button', { name: '正在采集' }));
    await userEvent.click(screen.getByRole('button', { name: '暂停抓取' }));

    await userEvent.click(screen.getByRole('button', { name: '加载数据' }));
    expect(onToggleRecord).toHaveBeenCalled();
    expect(onPause).toHaveBeenCalled();

    expect(onImport).toHaveBeenCalled();
  });

  it('未连接时 Record 与暂停不可用', () => {
    renderView({ connected: false, recording: false });
    expect(screen.getByRole('button', { name: '未采集' })).toBeDisabled();
    expect(screen.getByRole('button', { name: '暂停抓取' })).toBeDisabled();
  });
});

describe('ProfilerView 时间轴视口', () => {
  afterEach(() => localStorage.clear());

  const many = Array.from({ length: 400 }, (_, i) => frameAt(i));

  function viewport() {
    return screen.getByTestId('profiler-viewport');
  }

  it('跟随时窗口钉在最新 300 帧', () => {
    renderView({ frames: many });
    expect(viewport()).toHaveAttribute('data-follow', 'true');
    expect(viewport()).toHaveAttribute('data-visible', '300');
    expect(viewport()).toHaveAttribute('data-start', '100');
    expect(screen.getByLabelText('当前帧')).toHaveValue('400');
    expect(screen.getByTestId('profiler-range')).toHaveTextContent('101–400 · 300帧');
  });

  it('点选历史帧后停止跟随（视图暂停）', async () => {
    renderView({ frames: many });
    await userEvent.click(screen.getByRole('button', { name: '上一帧' }));
    expect(viewport()).toHaveAttribute('data-follow', 'false');
    expect(screen.getByLabelText('当前帧')).toHaveValue('399');
    expect(viewport()).toHaveAttribute('data-start', '100');
  });

  it('Current 恢复实时跟随并取消选中', async () => {
    renderView({ frames: many });
    await userEvent.click(screen.getByRole('button', { name: '上一帧' }));
    await userEvent.click(screen.getByRole('button', { name: '最新帧' }));
    expect(viewport()).toHaveAttribute('data-follow', 'true');
    expect(viewport()).toHaveAttribute('data-selected', '-1');
    expect(screen.getByLabelText('当前帧')).toHaveValue('');
    expect(screen.getByText('选择一帧以查看详情')).toBeInTheDocument();
  });

  it('跳到最新帧后帧数增长不再拉取详情', async () => {
    const getHierarchy = vi.fn().mockResolvedValue([]);
    const getFrame = vi.fn(async (i: number) => frameAt(i));
    const fetchRange = vi.fn(async (offset: number, limit: number) =>
      Array.from({ length: Math.min(limit, 400 - offset) }, (_, i) => frameAt(offset + i)),
    );
    const props: ComponentProps<typeof ProfilerView> = {
      recording: true,
      connected: true,
      onClear: noop,
      onToggleRecord: noop,
      onPause: noop,
      onImport: noop,
      getFrame,
      getHierarchy,
      fetchRange,
      totalCount: 10,
    };
    const { rerender } = render(<ProfilerView {...props} />);
    await waitFor(() => expect(getHierarchy).toHaveBeenCalled());
    await userEvent.click(screen.getByRole('button', { name: '最新帧' }));
    await waitFor(() => expect(screen.getByText('选择一帧以查看详情')).toBeInTheDocument());
    const calls = getHierarchy.mock.calls.length;
    rerender(<ProfilerView {...props} totalCount={20} />);
    await waitFor(() => expect(viewport()).toHaveAttribute('data-selected', '-1'));
    expect(getHierarchy.mock.calls.length).toBe(calls);
  });

  it('滚轮向下缩小（可见帧变多）且保持跟随', () => {
    renderView({ frames: many });
    const canvas = screen.getByTestId('profiler-chart-gpu').querySelector('.profiler-chart-canvas');
    fireEvent.wheel(canvas!, { deltaY: 120, clientX: 512 });
    expect(Number(viewport().getAttribute('data-visible'))).toBeGreaterThan(300);
    expect(viewport()).toHaveAttribute('data-follow', 'true');
  });

  it('滚轮向上能从默认的 300 帧继续放大，且保持跟随', () => {
    renderView({ frames: many });
    const canvas = screen.getByTestId('profiler-chart-gpu').querySelector('.profiler-chart-canvas');
    fireEvent.wheel(canvas!, { deltaY: -120, clientX: 512 });
    expect(viewport()).toHaveAttribute('data-visible', '240');
    expect(viewport()).toHaveAttribute('data-start', '160');
    expect(viewport()).toHaveAttribute('data-follow', 'true');
  });

  it('实时采集从少量帧增长到默认窗口，滚轮放大后保持新窗口', async () => {
    const fetchRange = vi.fn(async (offset: number, limit: number) => many.slice(offset, offset + limit));
    const props = { connected: true, recording: true, onClear: noop, onToggleRecord: noop, onPause: noop, onImport: noop, fetchRange };
    const { rerender } = render(<ProfilerView {...props} totalCount={10} />);
    await waitFor(() => expect(fetchRange).toHaveBeenLastCalledWith(0, 10));
    rerender(<ProfilerView {...props} totalCount={320} />);
    expect(viewport()).toHaveAttribute('data-visible', '300');
    const canvas = screen.getByTestId('profiler-chart-gpu').querySelector('.profiler-chart-canvas');
    fireEvent.wheel(canvas!, { deltaY: -120, clientX: 512 });
    rerender(<ProfilerView {...props} totalCount={400} />);
    expect(viewport()).toHaveAttribute('data-visible', '240');
    expect(viewport()).toHaveAttribute('data-start', '160');
    await waitFor(() => expect(fetchRange).toHaveBeenLastCalledWith(160, 240));
  });

  it('CPU Y 轴按可见帧自动缩放，可手动调整并双击恢复自动比例', () => {
    const spike = emptyFrame({
      frameIndex: 500,
      cpuTimeMs: 160,
      cpuScriptsMs: 120,
      cpuRenderingMs: 20,
      cpuPhysicsMs: 20,
    });
    renderView({ frames: [frameAt(0), spike] });
    const chart = screen.getByTestId('profiler-chart-cpu');
    const axis = screen.getByRole('slider', { name: '调整 CPU Usage 纵轴上限' });
    const defaultMax = Number(chart.getAttribute('data-y-max'));
    expect(defaultMax).toBeCloseTo(176);
    expect(chart).toHaveAttribute('data-overflow-count', '0');
    expect(axis).toHaveAttribute('aria-valuemin', '0.01');

    for (const target of [axis, chart.querySelector('.profiler-chart-canvas')!]) {
      for (const deltaY of [-120, 120]) {
        expect(fireEvent.wheel(target, { deltaY, cancelable: true })).toBe(true);
        expect(Number(chart.getAttribute('data-y-max'))).toBe(defaultMax);
      }
    }
    Object.defineProperty(axis, 'setPointerCapture', { value: vi.fn() });
    fireEvent.pointerDown(axis, { pointerId: 1, clientY: 100 });
    fireEvent.pointerMove(axis, { pointerId: 1, clientY: 0 });
    fireEvent.pointerUp(axis, { pointerId: 1 });
    const manualMax = Number(chart.getAttribute('data-y-max'));
    expect(manualMax).toBeLessThan(defaultMax);
    expect(chart).toHaveAttribute('data-overflow-count', '1');
    expect(screen.queryByRole('status')).not.toBeInTheDocument();

    fireEvent.doubleClick(axis);
    expect(Number(chart.getAttribute('data-y-max'))).toBe(defaultMax);
    expect(chart).toHaveAttribute('data-overflow-count', '0');
  });

  it('CPU 图例可切换分类和通过拖放调整堆叠顺序', async () => {
    renderView({ frames: [emptyFrame({ cpuTimeMs: 100, cpuScriptsMs: 92, cpuRenderingMs: 8 })] });
    const chart = screen.getByTestId('profiler-chart-cpu');
    const legend = within(chart);
    const scripts = legend.getByRole('button', { name: 'Scripts' });
    const rendering = legend.getByRole('button', { name: 'Rendering' });
    expect(chart).toHaveAttribute('data-y-max', '110.00');
    await userEvent.click(scripts);
    expect(scripts).toHaveAttribute('aria-pressed', 'false');
    expect(chart).toHaveAttribute('data-y-max', '33.40');
    const dataTransfer = { setData: vi.fn(), effectAllowed: '' };
    fireEvent.dragStart(scripts, { dataTransfer });
    fireEvent.dragOver(rendering, { dataTransfer });
    fireEvent.drop(rendering, { dataTransfer });
    expect(legend.getAllByRole('button', { pressed: false })[0]).toBe(scripts);
    expect(within(legend.getByRole('list')).getAllByRole('button').slice(0, 2)).toEqual([scripts, rendering]);
    await userEvent.click(scripts);
    expect(scripts).toHaveAttribute('aria-pressed', 'true');
    expect(chart).toHaveAttribute('data-y-max', '110.00');
  });

  it('GPU Usage 与 Rendering 超过 100 时标记溢出而不扩展纵轴', () => {
    const spike = emptyFrame({ gpuTimeMs: 160, drawCalls: 160 });
    renderView({ frames: [spike] });
    expect(screen.getByTestId('profiler-chart-gpu')).toHaveAttribute('data-overflow-count', '1');
    expect(screen.getByTestId('profiler-chart-rendering')).toHaveAttribute('data-overflow-count', '1');
  });

  it('GPU Usage 与 Rendering 也可独立调整纵轴并恢复 100', () => {
    renderView();
    for (const label of ['调整 GPU Usage 纵轴上限', '调整 Rendering 纵轴上限']) {
      const axis = screen.getByRole('slider', { name: label });
      fireEvent.wheel(axis, { deltaY: 100 });
      expect(axis).not.toHaveAttribute('aria-valuenow', '100');
      fireEvent.doubleClick(axis);
      expect(axis).toHaveAttribute('aria-valuenow', '100');
    }
  });

  it('滚动条平移会暂停跟随', () => {
    renderView({ frames: many });
    const bar = screen.getByLabelText('时间轴滚动');
    expect(bar).toBeEnabled();
    fireEvent.change(bar, { target: { value: '10' } });
    expect(viewport()).toHaveAttribute('data-start', '10');
    expect(viewport()).toHaveAttribute('data-follow', 'false');
  });

  it('帧数不足一屏时滚动条禁用', () => {
    renderView();
    expect(screen.getByLabelText('时间轴滚动')).toBeDisabled();
    expect(screen.getByTestId('profiler-range')).toHaveTextContent('1–3 · 3帧');
  });

  it('切换模块时详情面板高度保持不变，拖拽可调高', async () => {
    renderView();
    const wrap = screen.getByTestId('profiler-detail-wrap');
    const before = wrap.style.height;
    await userEvent.click(screen.getByLabelText('Memory'));
    expect(wrap.style.height).toBe(before);
    const handle = screen.getByTestId('profiler-resize');
    expect(wrap.contains(handle)).toBe(true);
    fireEvent.pointerDown(handle, { clientY: 400, pointerId: 1 });
    fireEvent.pointerMove(handle, { clientY: 300, pointerId: 1 });
    fireEvent.pointerUp(handle, { clientY: 300, pointerId: 1 });
    expect(Number.parseInt(wrap.style.height, 10)).toBeGreaterThan(Number.parseInt(before, 10) || 0);
  });

  it('Deep Profile 开关传到工具栏', async () => {
    const onDeepProfiling = vi.fn();
    renderView({ onDeepProfiling, connected: true });
    await userEvent.click(screen.getByLabelText('Deep Profile'));
    expect(onDeepProfiling).toHaveBeenCalledWith(true);
  });
});
