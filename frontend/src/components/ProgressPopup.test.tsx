import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProgressPopup } from './ProgressPopup';
import type { ImportProgressEvent } from '../types';

function progress(over: Partial<ImportProgressEvent> = {}): ImportProgressEvent {
  return {
    phase: 'parse',
    done: 4,
    total: 10,
    percent: 40,
    fileName: 'boss.data',
    ...over,
  };
}

describe('ProgressPopup', () => {
  it('保存时更新百分比，完成后显示路径并可确认', async () => {
    const path = 'D:\\captures\\boss.upac';
    const onClose = vi.fn();
    const { rerender } = render(<ProgressPopup progress={null} save={{ path, percent: 42 }} onClose={onClose} />);
    expect(screen.getByRole('progressbar', { name: '保存进度' })).toHaveAttribute('aria-valuenow', '42');
    expect(screen.getByText('boss.upac')).toBeInTheDocument();
    expect(screen.queryByText('保存完成')).not.toBeInTheDocument();

    rerender(<ProgressPopup progress={null} save={{ path, percent: 100 }} onClose={onClose} />);
    expect(screen.getByRole('dialog', { name: '保存完成' })).toHaveAttribute('aria-busy', 'false');
    expect(screen.getByRole('status')).toHaveTextContent(path);
    await userEvent.click(screen.getByRole('button', { name: '确定' }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('Unity Editor 导出期间显示动态进度条', () => {
    render(<ProgressPopup progress={null} save={{ path: 'boss.data', percent: null }} />);
    expect(screen.getByRole('dialog', { name: 'Unity 正在导出 .data…' })).toBeInTheDocument();
    expect(screen.getByRole('progressbar', { name: '保存进度' })).not.toHaveAttribute('aria-valuenow');
  });
  it('无进度时不渲染', () => {
    render(<ProgressPopup progress={null} />);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('展示解析文案、文件名与百分比', () => {
    render(<ProgressPopup progress={progress()} />);
    expect(screen.getByRole('dialog', { name: '正在解析 Profiler 数据…' })).toBeInTheDocument();
    expect(screen.getByText('boss.data')).toBeInTheDocument();
    expect(screen.getByText('40%')).toBeInTheDocument();
    expect(screen.getByRole('progressbar', { name: '解析进度' })).toHaveAttribute('aria-valuenow', '40');
  });

  it('读取阶段显示读取提示', () => {
    render(<ProgressPopup progress={progress({ phase: 'read', percent: 0 })} />);
    expect(screen.getByRole('dialog', { name: '正在读取存档…' })).toBeInTheDocument();
  });
});
