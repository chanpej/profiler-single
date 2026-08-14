import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { FrameTable } from './FrameTable';
import { emptyFrame } from '../lib/frame';
import type { FrameData } from '../types';

function makeFrame(i: number): FrameData {
  return emptyFrame({
    frameIndex: i,
    timestampMs: 1755072000000 + i * 16,
    fps: 60,
    cpuTimeMs: 16.6,
    gpuTimeMs: 12,
    mainThreadMs: 14,
    renderMs: 6,
    gcAllocKB: 4,
    usedMemMB: 512,
    reservedMB: 700,
    drawCalls: 300,
    setPassCalls: 50,
    batches: 240,
    triangles: 900000,
    vertices: 660000,
  });
}

describe('FrameTable', () => {
  it('渲染表头', () => {
    render(<FrameTable totalCount={0} fetchRange={vi.fn().mockResolvedValue([])} />);
    expect(screen.getByText('帧号')).toBeInTheDocument();
    expect(screen.getByText('DrawCall')).toBeInTheDocument();
  });

  it('懒加载并渲染可视区域数据', async () => {
    const fetchRange = vi.fn((offset: number, limit: number) =>
      Promise.resolve(Array.from({ length: Math.min(limit, 1000 - offset) }, (_, i) => makeFrame(offset + i))),
    );
    render(<FrameTable totalCount={1000} fetchRange={fetchRange} />);

    await waitFor(() => {
      expect(screen.getByText('#0')).toBeInTheDocument();
    });
    expect(fetchRange).toHaveBeenCalledWith(0, 200);
    // 单元格数值渲染正确。
    expect(screen.getAllByText('16.6ms').length).toBeGreaterThan(0);
    expect(screen.getAllByText('900.0k').length).toBeGreaterThan(0);
  });

  it('同一数据块不重复拉取', async () => {
    const fetchRange = vi.fn().mockResolvedValue(Array.from({ length: 200 }, (_, i) => makeFrame(i)));
    const { rerender } = render(<FrameTable totalCount={200} fetchRange={fetchRange} />);
    await waitFor(() => expect(screen.getByText('#0')).toBeInTheDocument());
    rerender(<FrameTable totalCount={200} fetchRange={fetchRange} />);
    await waitFor(() => expect(screen.getByText('#0')).toBeInTheDocument());
    expect(fetchRange).toHaveBeenCalledTimes(1);
  });
});
