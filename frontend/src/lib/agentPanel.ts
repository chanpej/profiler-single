const WIDTH_KEY = 'upa.agentPanelWidth';
const COLLAPSED_KEY = 'upa.agentPanelCollapsed';

export const DEFAULT_AGENT_WIDTH = 380;
export const MIN_AGENT_WIDTH = 280;
export const MAX_AGENT_WIDTH = 720;
export const COLLAPSED_AGENT_WIDTH = 32;

/** 把 Agent 面板宽度限制在可拖拽范围内。 */
export function clampAgentWidth(w: number): number {
  if (!Number.isFinite(w)) return DEFAULT_AGENT_WIDTH;
  return Math.min(MAX_AGENT_WIDTH, Math.max(MIN_AGENT_WIDTH, Math.round(w)));
}

export function loadAgentWidth(): number {
  try {
    const raw = localStorage.getItem(WIDTH_KEY);
    if (!raw) return DEFAULT_AGENT_WIDTH;
    return clampAgentWidth(Number.parseInt(raw, 10));
  } catch {
    return DEFAULT_AGENT_WIDTH;
  }
}

export function saveAgentWidth(w: number): void {
  try {
    localStorage.setItem(WIDTH_KEY, String(clampAgentWidth(w)));
  } catch {
    // WebView 存储不可用时忽略。
  }
}

export function loadAgentCollapsed(): boolean {
  try {
    return localStorage.getItem(COLLAPSED_KEY) === '1';
  } catch {
    return false;
  }
}

export function saveAgentCollapsed(collapsed: boolean): void {
  try {
    localStorage.setItem(COLLAPSED_KEY, collapsed ? '1' : '0');
  } catch {
    // WebView 存储不可用时忽略。
  }
}

/** 根据拖拽起点计算新宽度（左边缘：向左拖变宽）。 */
export function widthFromDrag(startWidth: number, startX: number, clientX: number): number {
  return clampAgentWidth(startWidth + (startX - clientX));
}
