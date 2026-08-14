import { describe, expect, it, afterEach } from 'vitest';
import { clampDetailHeight, DEFAULT_DETAIL_HEIGHT, loadDetailHeight, MAX_DETAIL_HEIGHT, MIN_DETAIL_HEIGHT, saveDetailHeight } from './detailHeight';

describe('detailHeight', () => {
  afterEach(() => localStorage.clear());

  it('限制在最小/最大高度之间', () => {
    expect(clampDetailHeight(10)).toBe(MIN_DETAIL_HEIGHT);
    expect(clampDetailHeight(9999)).toBe(MAX_DETAIL_HEIGHT);
    expect(clampDetailHeight(Number.NaN)).toBe(DEFAULT_DETAIL_HEIGHT);
    expect(clampDetailHeight(200.8)).toBe(201);
  });

  it('持久化并恢复', () => {
    expect(loadDetailHeight()).toBe(DEFAULT_DETAIL_HEIGHT);
    saveDetailHeight(300);
    expect(loadDetailHeight()).toBe(300);
  });
});
