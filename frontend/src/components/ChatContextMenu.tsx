import { useEffect, useRef } from 'react';
import { clampMenuPos, copyText, elementPlainText, selectedText, selectElementText, type ChatContextMenuState } from '../lib/agentChatUi';

interface Props {
  menu: ChatContextMenuState;
  onClose: () => void;
}

/** 聊天消息右键菜单：复制选区、复制整条、全选。 */
export function ChatContextMenu({ menu, onClose }: Props) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    const rect = node.getBoundingClientRect();
    const pos = clampMenuPos(menu.x, menu.y, rect.width, rect.height, window.innerWidth, window.innerHeight);
    node.style.left = `${pos.x}px`;
    node.style.top = `${pos.y}px`;
  }, [menu.x, menu.y]);

  useEffect(() => {
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('mousedown', onDown);
    window.addEventListener('keydown', onKey);
    window.addEventListener('resize', onClose);
    window.addEventListener('scroll', onClose, true);
    return () => {
      window.removeEventListener('mousedown', onDown);
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('resize', onClose);
      window.removeEventListener('scroll', onClose, true);
    };
  }, [onClose]);

  const selection = selectedText();
  const messageText = menu.message ? elementPlainText(menu.message) : '';
  const canCopy = selection.trim().length > 0 || messageText.length > 0;

  return (
    <div
      ref={ref}
      className="agent-chat-context-menu"
      role="menu"
      data-testid="agent-chat-context-menu"
      style={{ left: menu.x, top: menu.y }}
    >
      <button
        type="button"
        role="menuitem"
        className="agent-chat-context-item"
        disabled={!canCopy}
        onClick={async () => {
          await copyText(selection || messageText);
          onClose();
        }}
      >
        复制
      </button>
      <button
        type="button"
        role="menuitem"
        className="agent-chat-context-item"
        disabled={!messageText}
        onClick={async () => {
          await copyText(messageText);
          onClose();
        }}
      >
        复制消息
      </button>
      <button
        type="button"
        role="menuitem"
        className="agent-chat-context-item"
        disabled={!menu.message}
        onClick={() => {
          if (menu.message) selectElementText(menu.message);
          onClose();
        }}
      >
        全选
      </button>
    </div>
  );
}
