import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { TitleBar } from './TitleBar';
import type { WindowControls } from '../lib/window';

function mockWin(over: Partial<WindowControls> = {}): WindowControls {
  return {
    minimise: vi.fn().mockResolvedValue(undefined),
    toggleMaximise: vi.fn().mockResolvedValue(undefined),
    close: vi.fn().mockResolvedValue(undefined),
    isMaximised: vi.fn().mockResolvedValue(false),
    ...over,
  };
}

describe('TitleBar', () => {
  it('显示标题与窗口按钮', () => {
    render(<TitleBar controls={mockWin()} />);
    expect(screen.getByTestId('titlebar')).toBeInTheDocument();
    expect(screen.getByTestId('titlebar-icon')).toHaveAttribute('src', '/app-icon.png');
    expect(screen.getByText('unityperf')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '最小化' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '最大化' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '关闭' })).toBeInTheDocument();
  });

  it('显示副标题', () => {
    render(<TitleBar subtitle="session.data" controls={mockWin()} />);
    expect(screen.getByText('session.data')).toBeInTheDocument();
  });

  it('最小化 / 最大化 / 关闭回调', async () => {
    const win = mockWin();
    render(<TitleBar controls={win} />);
    await userEvent.click(screen.getByRole('button', { name: '最小化' }));
    await userEvent.click(screen.getByRole('button', { name: '最大化' }));
    await userEvent.click(screen.getByRole('button', { name: '关闭' }));
    expect(win.minimise).toHaveBeenCalled();
    expect(win.toggleMaximise).toHaveBeenCalled();
    expect(win.close).toHaveBeenCalled();
  });

  it('双击标题区域切换最大化', async () => {
    const win = mockWin();
    render(<TitleBar controls={win} />);
    await userEvent.dblClick(screen.getByText('unityperf'));
    expect(win.toggleMaximise).toHaveBeenCalled();
  });

  it('已最大化时显示还原', async () => {
    const win = mockWin({ isMaximised: vi.fn().mockResolvedValue(true) });
    render(<TitleBar controls={win} />);
    expect(await screen.findByRole('button', { name: '还原' })).toBeInTheDocument();
  });
});
