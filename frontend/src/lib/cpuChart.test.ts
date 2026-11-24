import { describe, expect, it, vi } from 'vitest';
import { drawCpuAreas } from './cpuChart';
import type { SeriesPoint } from './profiler';

const plot = { left: 42, bottom: 100, width: 90, height: 100, max: 100 };

function makeSeries(id: string, values: number[]): SeriesPoint {
  return { id, label: id, color: '#123456', unit: 'ms', values };
}

function mockContext() {
  const calls = { beginPath: vi.fn(), moveTo: vi.fn(), lineTo: vi.fn(), closePath: vi.fn(), fill: vi.fn(), fillRect: vi.fn() };
  return { calls, context: calls as unknown as CanvasRenderingContext2D };
}

describe('CPU 堆叠面积', () => {
  it('以帧中心连接采样点，上层从下层顶边开始堆叠', () => {
    const { calls, context } = mockContext();
    drawCpuAreas(context, [makeSeries('rendering', [10, 20, 10]), makeSeries('scripts', [5, 20, 2])], [20, 21, 22], plot);
    expect(calls.moveTo.mock.calls).toEqual([[57, 90], [57, 85]]);
    expect(calls.lineTo.mock.calls.slice(5)).toEqual([[87, 60], [117, 88], [117, 90], [87, 80], [57, 90]]);
    expect(calls.fill).toHaveBeenCalledTimes(2);
    expect(calls.fillRect).not.toHaveBeenCalled();
  });

  it('缺帧处断开面积，孤立帧仍可见', () => {
    const { calls, context } = mockContext();
    drawCpuAreas(context, [makeSeries('scripts', [10, 20, 15])], [20, 21, 24], plot);
    expect(calls.lineTo.mock.calls).toEqual([[87, 80], [87, 100], [57, 100]]);
    expect(calls.fillRect).toHaveBeenCalledWith(107, 85, 20, 15);
  });
});
