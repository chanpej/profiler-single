import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AppDialog } from './AppDialog';

describe('AppDialog', () => {
  it('关闭时不渲染', () => {
    render(
      <AppDialog title="设置" open={false} onClose={vi.fn()}>
        <p>内容</p>
      </AppDialog>,
    );
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('打开时展示标题与内容', () => {
    render(
      <AppDialog title="诊断" open onClose={vi.fn()}>
        <p>报告</p>
      </AppDialog>,
    );
    expect(screen.getByRole('dialog', { name: '诊断' })).toBeInTheDocument();
    expect(screen.getByText('报告')).toBeInTheDocument();
  });

  it('点击关闭按钮触发 onClose', async () => {
    const onClose = vi.fn();
    render(
      <AppDialog title="帧数据" open onClose={onClose}>
        <p>表</p>
      </AppDialog>,
    );
    await userEvent.click(screen.getByRole('button', { name: '关闭对话框' }));
    expect(onClose).toHaveBeenCalled();
  });

  it('Escape 关闭对话框', async () => {
    const onClose = vi.fn();
    render(
      <AppDialog title="设置" open onClose={onClose}>
        <p>配置</p>
      </AppDialog>,
    );
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalled();
  });
});
