import { describe, expect, it, vi, afterEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfilerView } from './ProfilerView';
import { emptyFrame } from '../lib/frame';
import type { FrameData } from '../types';
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

  it('CPU Timeline 页签可切换', async () => {
    renderView();
    await userEvent.click(screen.getByRole('tab', { name: 'Timeline' }));
    expect(screen.getByTestId('cpu-timeline')).toBeInTheDocument();
    expect(screen.getByText('Main Thread')).toBeInTheDocument();
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
    const canvas = screen.getByTestId('profiler-chart-cpu').querySelector('.profiler-chart-canvas');
    fireEvent.wheel(canvas!, { deltaY: 120, clientX: 512 });
    expect(Number(viewport().getAttribute('data-visible'))).toBeGreaterThan(300);
    expect(viewport()).toHaveAttribute('data-follow', 'true');
  });

  it('滚轮向上放大不低于 300 帧', () => {
    renderView({ frames: many });
    const canvas = screen.getByTestId('profiler-chart-cpu').querySelector('.profiler-chart-canvas');
    fireEvent.wheel(canvas!, { deltaY: -120, clientX: 512 });
    expect(viewport()).toHaveAttribute('data-visible', '300');
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
