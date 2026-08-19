import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  clampMenuPos,
  contextMenuFromEvent,
  copyText,
  elementPlainText,
  messageRootFrom,
  selectedText,
  selectElementText,
} from './agentChatUi';

describe('agentChatUi', () => {
  afterEach(() => {
    document.body.innerHTML = '';
    vi.restoreAllMocks();
    window.getSelection()?.removeAllRanges();
  });

  it('messageRootFrom 定位 assistant-ui 消息节点', () => {
    const item = document.createElement('div');
    item.className = 'agent-chat-message agent-chat-message-assistant';
    const msg = document.createElement('div');
    msg.className = 'agent-chat-message-text';
    msg.textContent = '一段性能结论';
    item.appendChild(msg);
    document.body.appendChild(item);
    expect(messageRootFrom(msg.firstChild)).toBe(msg);
    expect(elementPlainText(msg)).toBe('一段性能结论');
  });

  it('selectElementText 与 selectedText', () => {
    const msg = document.createElement('div');
    msg.className = 'agent-chat-message';
    msg.textContent = '可选中';
    document.body.appendChild(msg);
    selectElementText(msg);
    expect(selectedText()).toBe('可选中');
  });

  it('copyText 写入剪贴板', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
    expect(await copyText('  abc  ')).toBe(true);
    expect(writeText).toHaveBeenCalledWith('  abc');
    expect(await copyText('   ')).toBe(false);
  });

  it('clampMenuPos 避免超出视口', () => {
    expect(clampMenuPos(900, 700, 120, 80, 800, 600)).toEqual({ x: 676, y: 516 });
    expect(clampMenuPos(-10, -10, 120, 80, 800, 600)).toEqual({ x: 4, y: 4 });
  });

  it('contextMenuFromEvent 只在消息区弹出', () => {
    const viewport = document.createElement('div');
    viewport.className = 'agent-chat-viewport';
    const msg = document.createElement('div');
    msg.className = 'agent-chat-message';
    msg.textContent = 'hi';
    viewport.appendChild(msg);
    const ta = document.createElement('textarea');
    document.body.append(viewport, ta);
    expect(contextMenuFromEvent({ target: msg, clientX: 10, clientY: 20 })).toEqual({
      x: 10, y: 20, message: msg,
    });
    expect(contextMenuFromEvent({ target: ta, clientX: 1, clientY: 1 })).toBeNull();
  });
});
