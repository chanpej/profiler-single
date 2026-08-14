import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { IssuesPanel } from './IssuesPanel';
import type { Issue } from '../types';

describe('IssuesPanel', () => {
  it('无问题时显示空状态', () => {
    render(<IssuesPanel issues={[]} />);
    expect(screen.getByTestId('issues-empty')).toBeInTheDocument();
  });

  it('渲染问题列表及严重级别样式', () => {
    const issues: Issue[] = [
      { rule: 'frame_spike', severity: 'critical', title: '帧耗时尖峰', detail: '第 50 帧 CPU 耗时 120ms' },
      { rule: 'gc_spike', severity: 'warning', title: 'GC 分配尖峰', detail: '第 10 帧 GC 分配 1024KB' },
    ];
    render(<IssuesPanel issues={issues} />);
    expect(screen.getByText('帧耗时尖峰')).toBeInTheDocument();
    expect(screen.getByText('GC 分配尖峰')).toBeInTheDocument();
    const list = screen.getByTestId('issues-list');
    expect(list.querySelector('.sev-critical')).not.toBeNull();
    expect(list.querySelector('.sev-warning')).not.toBeNull();
  });
});
