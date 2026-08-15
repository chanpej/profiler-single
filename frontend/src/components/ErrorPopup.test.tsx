import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ErrorPopup } from './ErrorPopup';

describe('ErrorPopup', () => {
  it('关闭时不渲染', () => {
    render(<ErrorPopup message="boom" open={false} onClose={vi.fn()} />);
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });

  it('打开时展示错误内容', () => {
    render(<ErrorPopup message="采集中断" open onClose={vi.fn()} />);
    expect(screen.getByRole('alertdialog', { name: '错误' })).toBeInTheDocument();
    expect(screen.getByText('采集中断')).toBeInTheDocument();
  });

  it('点击关闭触发 onClose', async () => {
    const onClose = vi.fn();
    render(<ErrorPopup message="x" open onClose={onClose} />);
    await userEvent.click(screen.getByRole('button', { name: '关闭错误' }));
    expect(onClose).toHaveBeenCalled();
  });

  it('Escape 关闭', async () => {
    const onClose = vi.fn();
    render(<ErrorPopup message="x" open onClose={onClose} />);
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalled();
  });
});
