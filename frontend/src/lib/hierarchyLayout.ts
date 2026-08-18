const OVERVIEW_KEY = 'upa.hierarchyOverviewWidth';
const CALLSTACK_KEY = 'upa.callstackPaneRatio';

export const DEFAULT_OVERVIEW_WIDTH = 320;
export const MIN_OVERVIEW_WIDTH = 140;
export const MAX_OVERVIEW_WIDTH = 720;

export const DEFAULT_CALLSTACK_RATIO = 0.32;
export const MIN_CALLSTACK_RATIO = 0.16;
export const MAX_CALLSTACK_RATIO = 0.6;

export const METRIC_HEADERS = ['Total', 'Self', 'Calls', 'GC Alloc', 'Time ms', 'Self ms'] as const;
export const HIERARCHY_UI_FONT = "500 12px Inter, 'Segoe UI', system-ui, sans-serif";
export const HIERARCHY_MONO_FONT = "12px 'Cascadia Code', Menlo, Consolas, monospace";
/** 单元格左右 padding 8px × 2，再留一点余量避免贴边。 */
export const METRIC_CELL_PAD = 20;

let measureCanvas: HTMLCanvasElement | undefined;

/** 按实际字体测量文本宽度；jsdom 无排版时按字符估算。 */
export function measureTextWidth(text: string, font: string): number {
  if (typeof document !== 'undefined') {
    measureCanvas ??= document.createElement('canvas');
    const ctx = measureCanvas.getContext('2d');
    if (ctx) {
      ctx.font = font;
      const w = ctx.measureText(text).width;
      if (w > 0) return w;
    }
  }
  return text.length * 7;
}

/**
 * 数值列按表头与单元格中最宽文本决定宽度，Overview 才能吃掉表格剩余空间。
 */
export function metricColumnWidths(
  headers: readonly string[],
  valueRows: string[][],
  measure: (text: string, font: string) => number = measureTextWidth,
): number[] {
  return headers.map((h, i) => {
    let w = measure(h, HIERARCHY_UI_FONT);
    for (const row of valueRows) {
      w = Math.max(w, measure(row[i] ?? '', HIERARCHY_MONO_FONT));
    }
    return Math.max(METRIC_CELL_PAD, Math.ceil(w + METRIC_CELL_PAD));
  });
}

/** Overview 列宽限制在可拖拽范围内。 */
export function clampOverviewWidth(w: number): number {
  if (!Number.isFinite(w)) return DEFAULT_OVERVIEW_WIDTH;
  return Math.min(MAX_OVERVIEW_WIDTH, Math.max(MIN_OVERVIEW_WIDTH, Math.round(w)));
}

export function loadOverviewWidth(): number {
  try {
    const raw = localStorage.getItem(OVERVIEW_KEY);
    if (!raw) return DEFAULT_OVERVIEW_WIDTH;
    return clampOverviewWidth(Number.parseInt(raw, 10));
  } catch {
    return DEFAULT_OVERVIEW_WIDTH;
  }
}

export function saveOverviewWidth(w: number): void {
  try {
    localStorage.setItem(OVERVIEW_KEY, String(clampOverviewWidth(w)));
  } catch {
    // WebView 存储不可用时忽略。
  }
}

/** Overview 列：向右拖变宽。 */
export function overviewWidthFromDrag(startWidth: number, startX: number, clientX: number): number {
  return clampOverviewWidth(startWidth + (clientX - startX));
}

/** Call Stacks 面板宽度占比限制。 */
export function clampCallstackRatio(ratio: number): number {
  if (!Number.isFinite(ratio)) return DEFAULT_CALLSTACK_RATIO;
  return Math.min(MAX_CALLSTACK_RATIO, Math.max(MIN_CALLSTACK_RATIO, ratio));
}

export function loadCallstackRatio(): number {
  try {
    const raw = localStorage.getItem(CALLSTACK_KEY);
    if (!raw) return DEFAULT_CALLSTACK_RATIO;
    return clampCallstackRatio(Number.parseFloat(raw));
  } catch {
    return DEFAULT_CALLSTACK_RATIO;
  }
}

export function saveCallstackRatio(ratio: number): void {
  try {
    localStorage.setItem(CALLSTACK_KEY, String(clampCallstackRatio(ratio)));
  } catch {
    // WebView 存储不可用时忽略。
  }
}

/**
 * 按分隔条指针位置计算 Call Stacks 占比。
 * 分隔条在右侧面板左缘：向左拖，Call Stacks 变宽。
 */
export function callstackRatioFromPointer(
  containerWidth: number,
  containerLeft: number,
  clientX: number,
): number {
  if (!(containerWidth > 0)) return DEFAULT_CALLSTACK_RATIO;
  return clampCallstackRatio((containerWidth - (clientX - containerLeft)) / containerWidth);
}
