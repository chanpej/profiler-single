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
    render(<AnalysisView analyze={analyze} diagnose={vi.fn()} llmReady={false} />);

    await userEvent.click(screen.getByRole('button', { name: '执行规则分析' }));
    await waitFor(() => expect(screen.getByTestId('report-summary')).toBeInTheDocument());
    expect(screen.getByText('GC 分配尖峰')).toBeInTheDocument();
  });

  it('LLM 未配置时禁用诊断按钮并给出提示', () => {
    render(<AnalysisView analyze={vi.fn()} diagnose={vi.fn()} llmReady={false} />);
    expect(screen.getByRole('button', { name: 'LLM 诊断' })).toBeDisabled();
    expect(screen.getByText(/尚未配置 ACP Agent/)).toBeInTheDocument();
  });

  it('执行 LLM 诊断并展示结论', async () => {
    const diagnose = vi.fn().mockResolvedValue('诊断结论：GC 分配过高，建议使用对象池。');
    render(<AnalysisView analyze={vi.fn()} diagnose={diagnose} llmReady={true} />);

    await userEvent.click(screen.getByRole('button', { name: 'LLM 诊断' }));
    await waitFor(() => expect(screen.getByTestId('diagnosis')).toBeInTheDocument());
    expect(screen.getByTestId('diagnosis').textContent).toContain('对象池');
  });

  it('诊断失败时展示错误', async () => {
    const diagnose = vi.fn().mockRejectedValue(new Error('agent not found'));
    render(<AnalysisView analyze={vi.fn()} diagnose={diagnose} llmReady={true} />);

    await userEvent.click(screen.getByRole('button', { name: 'LLM 诊断' }));
    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
    expect(screen.getByRole('alert').textContent).toContain('agent not found');
  });
});
