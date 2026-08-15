import { afterEach, describe, expect, it } from 'vitest';
import {
  callstackRatioFromPointer,
  clampCallstackRatio,
  clampOverviewWidth,
  DEFAULT_CALLSTACK_RATIO,
  DEFAULT_OVERVIEW_WIDTH,
  loadCallstackRatio,
  loadOverviewWidth,
  MAX_CALLSTACK_RATIO,
  MAX_OVERVIEW_WIDTH,
  METRIC_CELL_PAD,
  metricColumnWidths,
  MIN_CALLSTACK_RATIO,
  MIN_OVERVIEW_WIDTH,
  overviewWidthFromDrag,
  saveCallstackRatio,
  saveOverviewWidth,
} from './hierarchyLayout';

describe('hierarchyLayout', () => {
  afterEach(() => localStorage.clear());

  it('限制 Overview 列宽', () => {
    expect(clampOverviewWidth(10)).toBe(MIN_OVERVIEW_WIDTH);
    expect(clampOverviewWidth(9999)).toBe(MAX_OVERVIEW_WIDTH);
    expect(clampOverviewWidth(Number.NaN)).toBe(DEFAULT_OVERVIEW_WIDTH);
    expect(clampOverviewWidth(200.8)).toBe(201);
  });

  it('持久化 Overview 列宽', () => {
    expect(loadOverviewWidth()).toBe(DEFAULT_OVERVIEW_WIDTH);
    saveOverviewWidth(400);
    expect(loadOverviewWidth()).toBe(400);
  });

  it('向右拖拽增加 Overview 列宽', () => {
    expect(overviewWidthFromDrag(320, 100, 180)).toBe(400);
    expect(overviewWidthFromDrag(320, 100, -200)).toBe(MIN_OVERVIEW_WIDTH);
  });

  it('限制 Call Stacks 宽度占比', () => {
    expect(clampCallstackRatio(0.01)).toBe(MIN_CALLSTACK_RATIO);
    expect(clampCallstackRatio(0.99)).toBe(MAX_CALLSTACK_RATIO);
    expect(clampCallstackRatio(Number.NaN)).toBe(DEFAULT_CALLSTACK_RATIO);
  });

  it('持久化 Call Stacks 占比', () => {
    expect(loadCallstackRatio()).toBe(DEFAULT_CALLSTACK_RATIO);
    saveCallstackRatio(0.4);
    expect(loadCallstackRatio()).toBe(0.4);
  });

  it('向左拖分隔条增加 Call Stacks 占比', () => {
    expect(callstackRatioFromPointer(1000, 0, 700)).toBe(0.3);
    expect(callstackRatioFromPointer(1000, 0, 100)).toBe(MAX_CALLSTACK_RATIO);
    expect(callstackRatioFromPointer(0, 0, 100)).toBe(DEFAULT_CALLSTACK_RATIO);
  });

  it('数值列宽度取表头与单元格中的较宽者', () => {
    const widths = metricColumnWidths(
      ['Total', 'GC Alloc'],
      [['1.0ms', '—'], ['16.0ms', '12.3 KB']],
      (text) => text.length,
    );
    expect(widths[0]).toBe('16.0ms'.length + METRIC_CELL_PAD);
    expect(widths[1]).toBe('GC Alloc'.length + METRIC_CELL_PAD);
  });
});
