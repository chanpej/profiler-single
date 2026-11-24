import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AnalysisView } from './AnalysisView';
import type { Report } from '../types';

const report: Report = {
  frameCount: 100,
  issues: [{ rule: 'gc_spike', severity: 'warning', title: 'GC 分配尖峰', detail: '第 10 帧分配 1024KB' }],
  summary: '分析了 100 帧，发现 1 个问题（严重 0，警告 1，提示 0）。',
};

describe('AnalysisView', () => {
  it('执行规则分析并展示报告', async () => {
    const analyze = vi.fn().mockResolvedValue(report);
    render(<AnalysisView analyze={analyze} />);

    await userEvent.click(screen.getByRole('button', { name: '执行规则分析' }));
    await waitFor(() => expect(screen.getByTestId('report-summary')).toBeInTheDocument());
    expect(screen.getByText('GC 分配尖峰')).toBeInTheDocument();
  });

  it('仅提供本地规则分析', () => {
    render(<AnalysisView analyze={vi.fn()} />);
    expect(screen.getAllByRole('button')).toEqual([screen.getByRole('button', { name: '执行规则分析' })]);
  });

  it('规则分析失败时展示错误', async () => {
    const analyze = vi.fn().mockRejectedValue(new Error('读取帧数据失败'));
    render(<AnalysisView analyze={analyze} />);

    await userEvent.click(screen.getByRole('button', { name: '执行规则分析' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('读取帧数据失败');
  });
});
