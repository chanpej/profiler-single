const STORAGE_KEY = 'upa.detailHeight';

export const DEFAULT_DETAIL_HEIGHT = 220;
export const MIN_DETAIL_HEIGHT = 140;
export const MAX_DETAIL_HEIGHT = 520;

/** 把详情面板高度限制在可拖拽范围内。 */
export function clampDetailHeight(h: number): number {
  if (!Number.isFinite(h)) return DEFAULT_DETAIL_HEIGHT;
  return Math.min(MAX_DETAIL_HEIGHT, Math.max(MIN_DETAIL_HEIGHT, Math.round(h)));
}

/** 读取上次拖拽保存的详情面板高度。 */
export function loadDetailHeight(): number {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULT_DETAIL_HEIGHT;
    return clampDetailHeight(Number.parseInt(raw, 10));
  } catch {
    return DEFAULT_DETAIL_HEIGHT;
  }
}

/** 持久化详情面板高度。 */
export function saveDetailHeight(h: number): void {
  try {
    localStorage.setItem(STORAGE_KEY, String(clampDetailHeight(h)));
  } catch {
    // WebView 存储不可用时忽略。
  }
}
