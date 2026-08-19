const FONT_KEY = 'upa.agentChatFontSize';

export const DEFAULT_CHAT_FONT_SIZE = 12;
export const MIN_CHAT_FONT_SIZE = 11;
export const MAX_CHAT_FONT_SIZE = 16;

/** 把聊天字号限制在可设置范围内。 */
export function clampChatFontSize(n: number): number {
  if (!Number.isFinite(n)) return DEFAULT_CHAT_FONT_SIZE;
  return Math.min(MAX_CHAT_FONT_SIZE, Math.max(MIN_CHAT_FONT_SIZE, Math.round(n)));
}

export function loadChatFontSize(): number {
  try {
    const raw = localStorage.getItem(FONT_KEY);
    if (!raw) return DEFAULT_CHAT_FONT_SIZE;
    return clampChatFontSize(Number.parseInt(raw, 10));
  } catch {
    return DEFAULT_CHAT_FONT_SIZE;
  }
}

export function saveChatFontSize(n: number): void {
  try {
    localStorage.setItem(FONT_KEY, String(clampChatFontSize(n)));
  } catch {
    // WebView 存储不可用时忽略。
  }
}

/** 把字号写到根节点 CSS 变量，聊天区与主界面共用。 */
export function applyChatFontSize(n: number): void {
  const px = `${clampChatFontSize(n)}px`;
  document.documentElement.style.setProperty('--agent-chat-font-size', px);
}
