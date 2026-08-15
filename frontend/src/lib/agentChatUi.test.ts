import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  clampMenuPos,
  clearComposerInput,
  composerIsBusy,
  contextMenuFromEvent,
  copyText,
  elementPlainText,
  messageRootFrom,
  selectedText,
  selectElementText,
  watchComposerSubmitClear,
} from './agentChatUi';

function composer(html = '<textarea>hello</textarea>') {
  const root = document.createElement('div');
  root.innerHTML = `<div class="nlux-comp-composer nlux-composer--typing">${html}</div>`;
  document.body.appendChild(root);
  return root;
}

describe('agentChatUi', () => {
  afterEach(() => {
    document.body.innerHTML = '';
    vi.restoreAllMocks();
    window.getSelection()?.removeAllRanges();
  });

  it('clearComposerInput 清空并派发 input', () => {
    const root = composer();
    const ta = root.querySelector('textarea') as HTMLTextAreaElement;
    const onInput = vi.fn();
    ta.addEventListener('input', onInput);
    expect(clearComposerInput(root)).toBe(true);
    expect(ta.value).toBe('');
    expect(onInput).toHaveBeenCalled();
    expect(clearComposerInput(root)).toBe(false);
  });

  it('composer 进入 submitting 时自动清空', async () => {
    const root = composer();
    const stop = watchComposerSubmitClear(root);
    const box = root.querySelector('.nlux-comp-composer') as HTMLElement;
    box.classList.remove('nlux-composer--typing');
    box.classList.add('nlux-composer--submitting');
    expect(composerIsBusy(box)).toBe(true);
    await vi.waitFor(() => {
      expect((root.querySelector('textarea') as HTMLTextAreaElement).value).toBe('');
    });
    stop();
  });

  it('messageRootFrom 定位消息节点', () => {
    const item = document.createElement('div');
    item.className = 'nlux-comp-chatItem';
    const msg = document.createElement('div');
    msg.className = 'nlux-comp-message';
    msg.textContent = '一段性能结论';
    item.appendChild(msg);
    document.body.appendChild(item);
    expect(messageRootFrom(msg.firstChild)).toBe(msg);
    expect(elementPlainText(msg)).toBe('一段性能结论');
  });

  it('selectElementText 与 selectedText', () => {
    const msg = document.createElement('div');
    msg.className = 'nlux-comp-message';
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
    const conv = document.createElement('div');
    conv.className = 'nlux-conversation-container';
    const msg = document.createElement('div');
    msg.className = 'nlux-comp-message';
    msg.textContent = 'hi';
    conv.appendChild(msg);
    const ta = document.createElement('textarea');
    document.body.append(conv, ta);
    expect(contextMenuFromEvent({ target: msg, clientX: 10, clientY: 20 })).toEqual({
      x: 10, y: 20, message: msg,
    });
    expect(contextMenuFromEvent({ target: ta, clientX: 1, clientY: 1 })).toBeNull();
  });
});
