import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
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
