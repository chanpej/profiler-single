export function selectedText(): string {
  return window.getSelection()?.toString() ?? '';
}

export function messageRootFrom(target: EventTarget | null): HTMLElement | null {
  if (!(target instanceof Node)) return null;
  const el = target instanceof Element ? target : target.parentElement;
  return el?.closest('.agent-chat-message, .agent-chat-message-text') as HTMLElement | null;
}

export function elementPlainText(el: HTMLElement): string {
  return (el.innerText || el.textContent || '').replace(/\s+\n/g, '\n').trim();
}

export function selectElementText(el: HTMLElement): void {
  const range = document.createRange();
  range.selectNodeContents(el);
  const sel = window.getSelection();
  sel?.removeAllRanges();
  sel?.addRange(range);
}

export async function copyText(text: string): Promise<boolean> {
  const value = text.trimEnd();
  if (!value) return false;
  try {
    await navigator.clipboard.writeText(value);
    return true;
  } catch {
    try {
      const node = document.createElement('textarea');
      node.value = value;
      node.setAttribute('readonly', '');
      node.style.position = 'fixed';
      node.style.left = '-9999px';
      document.body.appendChild(node);
      node.select();
      const ok = document.execCommand('copy');
      node.remove();
      return ok;
    } catch {
      return false;
    }
  }
}

export function clampMenuPos(
  x: number,
  y: number,
  menuW: number,
  menuH: number,
  vw: number,
  vh: number,
): { x: number; y: number } {
  return {
    x: Math.max(4, Math.min(x, vw - menuW - 4)),
    y: Math.max(4, Math.min(y, vh - menuH - 4)),
  };
}

export type ChatContextMenuState = {
  x: number;
  y: number;
  message: HTMLElement | null;
};

/** 仅在消息区域弹出菜单；输入框与工具栏交给系统处理。 */
export function contextMenuFromEvent(e: MouseEvent | { target: EventTarget | null; clientX: number; clientY: number }): ChatContextMenuState | null {
  const target = e.target;
  if (!(target instanceof Element)) return null;
  if (target.closest('textarea, input, select, button, .agent-composer, .agent-composer-tools')) {
    return null;
  }
  const message = messageRootFrom(target);
  const hasSelection = selectedText().trim().length > 0;
  if (!message && !hasSelection) return null;
  if (!target.closest('.agent-chat-viewport, .agent-chat-message')) return null;
  return { x: e.clientX, y: e.clientY, message };
}
