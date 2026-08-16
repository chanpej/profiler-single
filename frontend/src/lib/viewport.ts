/** Unity Profiler 风格时间轴视口：可见窗口、跟随最新帧、缩放与平移。 */

export const MIN_VISIBLE = 300;
export const MAX_VISIBLE = 4000;
export const DEFAULT_VISIBLE = 300;

export interface Viewport {
  /** 可见窗口第一帧（会话内绝对下标，0 为最旧保留帧）。 */
  start: number;
  /** 可见帧数，越小越放大。 */
  visible: number;
  /** 选中帧绝对下标；-1 表示未选中。 */
  selected: number;
  /** 为 true 时窗口钉在最新帧，随采集实时滚动。 */
  follow: boolean;
}

export function maxStart(total: number, visible: number): number {
  return Math.max(0, total - visible);
}

/** 可见窗口大小：不超过总数，至少覆盖全部（总数很小时）。 */
export function clampVisible(visible: number, total: number): number {
  if (total <= 0) {
    return Math.max(MIN_VISIBLE, Math.min(visible || DEFAULT_VISIBLE, MAX_VISIBLE));
  }
  const lo = Math.min(MIN_VISIBLE, total);
  const hi = Math.min(MAX_VISIBLE, total);
  return Math.max(lo, Math.min(visible, hi));
}

export function clampViewport(vp: Viewport, total: number): Viewport {
  if (total <= 0) {
    return { start: 0, visible: clampVisible(vp.visible, 0), selected: -1, follow: true };
  }
  const visible = clampVisible(vp.visible, total);
  const follow = vp.follow;
  const start = follow
    ? maxStart(total, visible)
    : Math.max(0, Math.min(vp.start, maxStart(total, visible)));
  let selected = vp.selected;
  if (selected < 0) {
    selected = -1;
  } else if (follow) {
    selected = total - 1;
  } else {
    selected = Math.max(0, Math.min(selected, total - 1));
  }
  return { start, visible, selected, follow };
}

/**
 * 缩放。factor < 1 放大（更少帧），> 1 缩小。
 * 跟随时钉住右缘（最新帧）；否则围绕 anchor 帧缩放。
 */
export function zoomViewport(vp: Viewport, total: number, factor: number, anchor: number): Viewport {
  const visible = clampVisible(Math.round(vp.visible * factor), total);
  if (visible === vp.visible) return clampViewport(vp, total);
  if (vp.follow) {
    return clampViewport({ ...vp, visible }, total);
  }
  const a = Math.max(0, Math.min(anchor, Math.max(0, total - 1)));
  const ratio = vp.visible <= 1 ? 0 : (a - vp.start) / vp.visible;
  const start = Math.round(a - ratio * visible);
  return clampViewport({ ...vp, start, visible }, total);
}

/** 平移可见窗口；会解除跟随，使视图停在当前位置。 */
export function panViewport(vp: Viewport, total: number, delta: number): Viewport {
  const cur = clampViewport(vp, total);
  return setViewportStart(cur, total, cur.start + delta);
}

/** 将窗口起点设为 start 并暂停跟随。 */
export function setViewportStart(vp: Viewport, total: number, start: number): Viewport {
  return clampViewport({ ...vp, start, follow: false }, total);
}

/** 按图表内相对下标选帧。选中最新帧则恢复跟随。 */
export function selectInView(vp: Viewport, total: number, localIndex: number): Viewport {
  if (total <= 0) return clampViewport(vp, total);
  const selected = Math.max(0, Math.min(vp.start + localIndex, total - 1));
  const follow = selected >= total - 1;
  return clampViewport({ ...vp, selected, follow }, total);
}

/** 跳到指定绝对帧，必要时平移窗口以保持可见。 */
export function seekTo(vp: Viewport, total: number, selected: number): Viewport {
  if (total <= 0) return clampViewport(vp, total);
  const s = Math.max(0, Math.min(selected, total - 1));
  let start = vp.start;
  if (s < start) start = s;
  else if (s >= start + vp.visible) start = s - vp.visible + 1;
  const follow = s >= total - 1;
  return clampViewport({ ...vp, selected: s, start, follow }, total);
}

/** 跳到最新帧并跟随，同时取消选中，避免详情面板跟着最新帧刷新。 */
export function goCurrent(vp: Viewport, total: number): Viewport {
  return clampViewport({ ...vp, follow: true, selected: -1 }, total);
}

/** 滚轮缩放系数：向上放大，向下缩小。 */
export function wheelZoomFactor(deltaY: number): number {
  if (deltaY === 0) return 1;
  return deltaY > 0 ? 1.25 : 1 / 1.25;
}

/** Shift+滚轮平移步长（约一成窗口）。 */
export function wheelPanDelta(deltaY: number, visible: number): number {
  const step = Math.max(1, Math.round(visible * 0.1));
  if (deltaY === 0) return 0;
  return deltaY > 0 ? step : -step;
}

/** 滚轮意图：Shift 或横向滚动为平移，否则为缩放。 */
export function wheelIntent(shiftKey: boolean, deltaX: number, _deltaY: number): 'pan' | 'zoom' {
  if (shiftKey || deltaX) return 'pan';
  return 'zoom';
}

export function localIndexFromX(x: number, width: number, visible: number): number {
  if (visible <= 0 || width <= 0) return 0;
  return Math.min(visible - 1, Math.max(0, Math.floor((x / width) * visible)));
}

export function selectedInWindow(vp: Viewport): number {
  if (vp.selected < vp.start || vp.selected >= vp.start + vp.visible) return -1;
  return vp.selected - vp.start;
}
