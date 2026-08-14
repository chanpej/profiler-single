import { describe, expect, it } from 'vitest';
import {
  DEFAULT_VISIBLE,
  clampViewport,
  clampVisible,
  goCurrent,
  localIndexFromX,
  maxStart,
  panViewport,
  seekTo,
  selectInView,
  selectedInWindow,
  setViewportStart,
  wheelPanDelta,
  wheelIntent,
  wheelZoomFactor,
  zoomViewport,
  type Viewport,
} from './viewport';

function vp(over: Partial<Viewport> = {}): Viewport {
  return { start: 0, visible: 300, selected: 0, follow: true, ...over };
}

describe('clampVisible', () => {
  it('总数小于默认窗口时可见帧等于总数', () => {
    expect(clampVisible(DEFAULT_VISIBLE, 3)).toBe(3);
    expect(clampVisible(300, 10)).toBe(10);
  });
  it('总数充足时限制在 300..4000', () => {
    expect(clampVisible(8, 1000)).toBe(300);
    expect(clampVisible(9000, 10000)).toBe(4000);
    expect(clampVisible(300, 1000)).toBe(300);
  });
});

describe('clampViewport follow', () => {
  it('跟随时窗口钉在末尾，并选中最新帧', () => {
    const got = clampViewport(vp({ follow: true, visible: 300, selected: 0 }), 500);
    expect(got.start).toBe(200);
    expect(got.visible).toBe(300);
    expect(got.selected).toBe(499);
  });
  it('采集增长时跟随窗口右移（实时滚动）', () => {
    let cur = clampViewport(vp({ follow: true, visible: 300, selected: 299 }), 300);
    expect(cur.start).toBe(0);
    cur = clampViewport({ ...cur, follow: true }, 400);
    expect(cur.start).toBe(100);
    expect(cur.selected).toBe(399);
  });
  it('非跟随在总数增长时保持 start（视图暂停）', () => {
    const paused = clampViewport(vp({ follow: false, start: 10, visible: 300, selected: 20 }), 400);
    expect(paused.start).toBe(10);
    expect(paused.follow).toBe(false);
    const later = clampViewport(paused, 800);
    expect(later.start).toBe(10);
  });
});

describe('zoomViewport', () => {
  it('跟随时放大到下限 300 仍钉住最新帧', () => {
    const got = zoomViewport(vp({ follow: true, visible: 600, selected: 999 }), 1000, 0.5, 800);
    expect(got.follow).toBe(true);
    expect(got.visible).toBe(300);
    expect(got.start).toBe(700);
  });
  it('非跟随围绕锚点缩放', () => {
    const got = zoomViewport(vp({ follow: false, start: 200, visible: 600, selected: 500 }), 1000, 0.5, 500);
    expect(got.visible).toBe(300);
    expect(got.start).toBe(350);
    expect(got.follow).toBe(false);
  });
  it('缩小到看全量', () => {
    const got = zoomViewport(vp({ follow: false, start: 50, visible: 300 }), 80, 8, 60);
    expect(got.visible).toBe(80);
    expect(got.start).toBe(0);
  });
  it('已达下限 300 时放大不再减少可见帧', () => {
    const got = zoomViewport(vp({ follow: true, visible: 300 }), 1000, 0.5, 900);
    expect(got.visible).toBe(300);
  });
});

describe('panViewport / select / seek', () => {
  it('平移解除跟随', () => {
    const got = panViewport(vp({ follow: true, visible: 300, selected: 499 }), 500, -10);
    expect(got.follow).toBe(false);
    expect(got.start).toBe(maxStart(500, 300) - 10);
  });
  it('选中非末帧暂停跟随', () => {
    const base = clampViewport(vp({ follow: true, visible: 300 }), 500);
    const got = selectInView(base, 500, 0);
    expect(got.follow).toBe(false);
    expect(got.selected).toBe(base.start);
  });
  it('选中窗口内最后一帧恢复跟随', () => {
    const base = clampViewport(vp({ follow: true, visible: 300 }), 500);
    const got = selectInView(base, 500, 299);
    expect(got.follow).toBe(true);
    expect(got.selected).toBe(499);
  });
  it('seek 超出窗口时带动平移', () => {
    const got = seekTo(vp({ follow: false, start: 200, visible: 300, selected: 250 }), 800, 20);
    expect(got.start).toBe(20);
    expect(got.selected).toBe(20);
    expect(got.follow).toBe(false);
  });
  it('Current 恢复跟随并跳到最新', () => {
    const got = goCurrent(vp({ follow: false, start: 0, visible: 300, selected: 3 }), 500);
    expect(got.follow).toBe(true);
    expect(got.selected).toBe(499);
    expect(got.start).toBe(200);
  });
  it('setViewportStart 暂停跟随', () => {
    const got = setViewportStart(vp({ follow: true, visible: 300, selected: 499 }), 500, 20);
    expect(got.follow).toBe(false);
    expect(got.start).toBe(20);
  });
});

describe('helpers', () => {
  it('滚轮方向：上放大下缩小', () => {
    expect(wheelZoomFactor(-100)).toBeLessThan(1);
    expect(wheelZoomFactor(100)).toBeGreaterThan(1);
  });
  it('Shift+滚轮或横向滚动视为平移', () => {
    expect(wheelIntent(true, 0, 100)).toBe('pan');
    expect(wheelIntent(false, 40, 0)).toBe('pan');
    expect(wheelIntent(false, 0, -100)).toBe('zoom');
  });
  it('Shift+滚轮平移步长', () => {
    expect(wheelPanDelta(100, 100)).toBe(10);
    expect(wheelPanDelta(-100, 100)).toBe(-10);
  });
  it('点击位置映射到窗口内下标', () => {
    expect(localIndexFromX(0, 100, 10)).toBe(0);
    expect(localIndexFromX(99, 100, 10)).toBe(9);
  });
  it('selectedInWindow 越界为 -1', () => {
    expect(selectedInWindow(vp({ start: 10, visible: 10, selected: 15 }))).toBe(5);
    expect(selectedInWindow(vp({ start: 10, visible: 10, selected: 3 }))).toBe(-1);
  });
});
