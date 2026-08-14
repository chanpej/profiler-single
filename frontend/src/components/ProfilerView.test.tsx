import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfilerView } from './ProfilerView';
import { emptyFrame } from '../lib/frame';
import type { FrameData } from '../types';

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

describe('ProfilerView', () => {
  it('默认展示 CPU/GPU/Rendering/Memory 模块图', () => {
    render(<ProfilerView frames={frames} recording onClear={vi.fn()} />);
    expect(screen.getByTestId('profiler-chart-cpu')).toBeInTheDocument();
    expect(screen.getByTestId('profiler-chart-memory')).toBeInTheDocument();
    expect(screen.queryByTestId('profiler-chart-audio')).not.toBeInTheDocument();
  });

  it('可开关 Profiler 模块', async () => {
    render(<ProfilerView frames={frames} recording onClear={vi.fn()} />);
    await userEvent.click(screen.getByLabelText('Audio'));
    expect(screen.getByTestId('profiler-chart-audio')).toBeInTheDocument();
    await userEvent.click(screen.getByLabelText('CPU Usage'));
    expect(screen.queryByTestId('profiler-chart-cpu')).not.toBeInTheDocument();
  });

  it('帧前进后退与 Current', async () => {
    render(<ProfilerView frames={frames} recording onClear={vi.fn()} />);
    expect(screen.getByLabelText('当前帧')).toHaveValue('2');
    await userEvent.click(screen.getByRole('button', { name: '上一帧' }));
    expect(screen.getByLabelText('当前帧')).toHaveValue('1');
    await userEvent.click(screen.getByRole('button', { name: '最新帧' }));
    expect(screen.getByLabelText('当前帧')).toHaveValue('2');
  });

  it('点击 Memory 模块后详情切换为 Simple 内存表', async () => {
    render(<ProfilerView frames={frames} recording onClear={vi.fn()} />);
    await userEvent.click(screen.getByLabelText('Memory'));
    expect(screen.getByTestId('memory-simple')).toBeInTheDocument();
    expect(screen.getByText('GfxDriver')).toBeInTheDocument();
  });

  it('Clear 按钮触发回调', async () => {
    const onClear = vi.fn();
    render(<ProfilerView frames={frames} recording={false} onClear={onClear} />);
    await userEvent.click(screen.getByRole('button', { name: '清空数据' }));
    expect(onClear).toHaveBeenCalled();
  });

  it('CPU Timeline 页签可切换', async () => {
    render(<ProfilerView frames={frames} recording onClear={vi.fn()} />);
    await userEvent.click(screen.getByRole('tab', { name: 'Timeline' }));
    expect(screen.getByTestId('cpu-timeline')).toBeInTheDocument();
    expect(screen.getByText('Main Thread')).toBeInTheDocument();
  });
});
