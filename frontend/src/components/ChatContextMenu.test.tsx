import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ChatContextMenu } from './ChatContextMenu';

describe('ChatContextMenu', () => {
  it('复制消息写入剪贴板后关闭', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
    const msg = document.createElement('div');
    msg.className = 'nlux-comp-message';
    msg.textContent = 'DrawCalls 过高';
    document.body.appendChild(msg);
    const onClose = vi.fn();
    render(<ChatContextMenu menu={{ x: 12, y: 24, message: msg }} onClose={onClose} />);
    await userEvent.click(screen.getByRole('menuitem', { name: '复制消息' }));
    expect(writeText).toHaveBeenCalledWith('DrawCalls 过高');
    expect(onClose).toHaveBeenCalled();
    msg.remove();
  });
});
