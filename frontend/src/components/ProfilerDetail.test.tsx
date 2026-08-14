import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfilerDetail } from './ProfilerDetail';
import { emptyFrame } from '../lib/frame';

const frame = emptyFrame({
  cpuTimeMs: 16.4,
  cpuRenderingMs: 5,
  cpuScriptsMs: 4,
  gpuTimeMs: 11,
  renderMs: 6,
  usedMemMB: 512,
  reservedMB: 700,
  gcUsedMB: 180,
  gcReservedMB: 240,
  gfxUsedMB: 160,
  drawCalls: 321,
  setPassCalls: 45,
  batches: 280,
  triangles: 1200000,
  playingSources: 9,
  audioVoices: 14,
  audioUsedMB: 8.5,
  activeRigidbodies: 42,
  colliders: 128,
  cpuPhysicsMs: 1.8,
  uiBatches: 24,
  uiVertices: 4800,
  cpuUiMs: 1.1,
  videoUsedMB: 0.5,
});

describe('ProfilerDetail', () => {
  it('CPU Hierarchy 列出 Rendering / Scripts', () => {
    render(<ProfilerDetail moduleId="cpu" frame={frame} />);
    expect(screen.getByText('Rendering')).toBeInTheDocument();
    expect(screen.getByText('Scripts')).toBeInTheDocument();
    expect(screen.getByText('5.0ms')).toBeInTheDocument();
  });

  it('无选中帧显示提示', () => {
    render(<ProfilerDetail moduleId="cpu" frame={null} />);
    expect(screen.getByText('选择一帧以查看详情')).toBeInTheDocument();
  });

  it('Memory Detailed 页签提示不支持快照', async () => {
    render(<ProfilerDetail moduleId="memory" frame={frame} />);
    await userEvent.click(screen.getByRole('tab', { name: 'Detailed' }));
    expect(screen.getByText(/内存快照/)).toBeInTheDocument();
  });

  it('Rendering / Audio / Physics / UI / GPU 计数器', () => {
    const { rerender } = render(<ProfilerDetail moduleId="rendering" frame={frame} />);
    expect(screen.getByText('SetPass Calls')).toBeInTheDocument();
    expect(screen.getByText('321')).toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="audio" frame={frame} />);
    expect(screen.getByText('Playing Sources')).toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="physics" frame={frame} />);
    expect(screen.getByText('Active Rigidbodies')).toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="ui" frame={frame} />);
    expect(screen.getByText('UI Batches')).toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="gpu" frame={frame} />);
    expect(screen.getByText('GPU Time')).toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="video" frame={frame} />);
    expect(screen.getByText('Video Memory')).toBeInTheDocument();
  });

  it('分类缺失帧：CPU Others 显示总耗时，Rendering 显示 Frame Time', () => {
    const coarse = emptyFrame({ fps: 59.9, cpuTimeMs: 34.3, usedMemMB: 700 });
    const { rerender } = render(<ProfilerDetail moduleId="cpu" frame={coarse} />);
    expect(screen.getByText('Others')).toBeInTheDocument();
    expect(screen.getByText('34.3ms')).toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="rendering" frame={coarse} />);
    expect(screen.getByText('Frame Time')).toBeInTheDocument();
  });
});
