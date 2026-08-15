/** 发送开始后立刻清空 NLUX 输入框。流式模式下库要等首个 chunk 才改状态，complete 时也不会清空。 */
export function clearComposerInput(root: ParentNode | null): boolean {
  if (!root) return false;
  const ta = root.querySelector('.nlux-comp-composer textarea');
  if (!(ta instanceof HTMLTextAreaElement)) return false;
  if (ta.value === '') return false;
  ta.value = '';
  ta.dispatchEvent(new Event('input', { bubbles: true }));
  ta.style.height = 'auto';
  return true;
}

export function composerIsBusy(el: Element | null): boolean {
  if (!el) return false;
  return el.classList.contains('nlux-composer--submitting')
    || el.classList.contains('nlux-composer--waiting');
}

/** 监听 composer 进入提交/等待，立刻清空输入。 */
export function watchComposerSubmitClear(
  root: HTMLElement,
  clear: (node: ParentNode | null) => boolean = clearComposerInput,
): () => void {
  let busy = false;
  const tick = () => {
    const composer = root.querySelector('.nlux-comp-composer');
    const now = composerIsBusy(composer);
    if (now && !busy) clear(root);
    busy = now;
  };
  tick();
  const obs = new MutationObserver(tick);
  obs.observe(root, {
    subtree: true,
    childList: true,
    attributes: true,
    attributeFilter: ['class'],
  });
  return () => obs.disconnect();
}

export function selectedText(): string {
  return window.getSelection()?.toString() ?? '';
}

export function messageRootFrom(target: EventTarget | null): HTMLElement | null {
  if (!(target instanceof Node)) return null;
  const el = target instanceof Element ? target : target.parentElement;
  return el?.closest('.nlux-comp-message, .nlux-markdown-container, .nlux-comp-chatItem') as HTMLElement | null;
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
  if (target.closest('textarea, input, select, button, .nlux-comp-composer, .agent-composer-tools')) {
    return null;
  }
  const message = messageRootFrom(target);
  const hasSelection = selectedText().trim().length > 0;
  if (!message && !hasSelection) return null;
  if (!target.closest('.nlux-conversation-container, .nlux-comp-message, .nlux-comp-chatItem')) {
    return null;
  }
  return { x: e.clientX, y: e.clientY, message };
}
