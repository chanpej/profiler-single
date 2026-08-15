import { afterEach, describe, expect, it } from 'vitest';
import {
  applyChatFontSize,
  clampChatFontSize,
  DEFAULT_CHAT_FONT_SIZE,
  loadChatFontSize,
  MAX_CHAT_FONT_SIZE,
  MIN_CHAT_FONT_SIZE,
  saveChatFontSize,
} from './chatAppearance';

describe('chatAppearance', () => {
  afterEach(() => {
    localStorage.clear();
    document.documentElement.style.removeProperty('--agent-chat-font-size');
  });

  it('限制字号范围', () => {
    expect(clampChatFontSize(8)).toBe(MIN_CHAT_FONT_SIZE);
    expect(clampChatFontSize(99)).toBe(MAX_CHAT_FONT_SIZE);
    expect(clampChatFontSize(Number.NaN)).toBe(DEFAULT_CHAT_FONT_SIZE);
    expect(clampChatFontSize(13.4)).toBe(13);
  });

  it('持久化并应用到 CSS 变量', () => {
    expect(loadChatFontSize()).toBe(DEFAULT_CHAT_FONT_SIZE);
    saveChatFontSize(14);
    expect(loadChatFontSize()).toBe(14);
    applyChatFontSize(14);
    expect(document.documentElement.style.getPropertyValue('--agent-chat-font-size')).toBe('14px');
  });
});
