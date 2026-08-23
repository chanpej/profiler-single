import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfilerToolbar } from './ProfilerToolbar';
import { emptyFrame } from '../lib/frame';

const frame = emptyFrame({ frameIndex: 10, cpuTimeMs: 16.4, fps: 59.9, gcAllocKB: 12.5 });
const noop = () => {};

function renderBar(over: Partial<Parameters<typeof ProfilerToolbar>[0]> = {}) {
  return render(
    <ProfilerToolbar
      recording
      connected
      frame={frame}
      selectedIndex={3}
      total={10}
      onClear={noop}
      onPrev={noop}
      onNext={noop}
      onSeek={noop}
      onCurrent={noop}
      onToggleRecord={noop}
      onPause={noop}

      onImport={noop}
      {...over}
    />,
  );
}

describe('ProfilerToolbar', () => {
  it('展示 Unity 风格 CPU / FPS / GC', () => {
    renderBar();
    expect(screen.getByText(/CPU 16.40ms/)).toBeInTheDocument();
    expect(screen.getByText(/59.9 FPS/)).toBeInTheDocument();
    expect(screen.getByText(/GC 12.5 KB/)).toBeInTheDocument();
  });

  it('暂停态显示继续抓取', () => {
    renderBar({ recording: false, connected: true });
    expect(screen.getByRole('button', { name: '继续抓取' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '未采集' })).toBeInTheDocument();
  });

  it('无数据时用图标表示无选中帧', () => {
    renderBar({ total: 0, selectedIndex: 0, frame: null });
    expect(screen.getByLabelText('无选中帧')).toBeInTheDocument();
    expect(screen.queryByText('无选中帧')).not.toBeInTheDocument();
  });

  it('Record / Clear / Load / Current 仅显示图标', () => {
    renderBar();
    expect(screen.queryByText('Record')).not.toBeInTheDocument();
    expect(screen.queryByText('Clear')).not.toBeInTheDocument();
    expect(screen.queryByText('Load')).not.toBeInTheDocument();

    expect(screen.queryByText('Current')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: '正在采集' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '清空数据' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '加载数据' })).toBeInTheDocument();

    expect(screen.getByRole('button', { name: '最新帧' })).toBeInTheDocument();
  });

  it('帧号按 Unity Profiler 从 1 计数', () => {
    renderBar({ selectedIndex: 3, total: 10 });
    expect(screen.getByLabelText('当前帧')).toHaveValue('4');
    expect(screen.getByText('/ 10')).toBeInTheDocument();
  });

  it('无选中帧时帧号为空且上一帧/下一帧不可用', () => {
    renderBar({ selectedIndex: -1, total: 10, frame: null });
    expect(screen.getByLabelText('当前帧')).toHaveValue('');
    expect(screen.getByRole('button', { name: '上一帧' })).toBeDisabled();
    expect(screen.getByRole('button', { name: '下一帧' })).toBeDisabled();
    expect(screen.getByLabelText('无选中帧')).toBeInTheDocument();
  });

  it('有选中帧时显示插入到 Agent Chat 的 AI 按钮', async () => {
    const onInsertFrameReference = vi.fn();
    const { rerender } = renderBar({ onInsertFrameReference });
    await userEvent.click(screen.getByRole('button', { name: '插入当前帧到 Agent Chat' }));
    expect(onInsertFrameReference).toHaveBeenCalledOnce();
    rerender(
      <ProfilerToolbar recording connected frame={null} selectedIndex={-1} total={10} onClear={noop} onPrev={noop} onNext={noop} onSeek={noop} onCurrent={noop} onToggleRecord={noop} onPause={noop} onImport={noop} />,
    );
    expect(screen.queryByRole('button', { name: '插入当前帧到 Agent Chat' })).not.toBeInTheDocument();
  });

  it('Load 触发回调', async () => {
    const onImport = vi.fn();
    renderBar({ onImport });
    await userEvent.click(screen.getByRole('button', { name: '加载数据' }));
    expect(onImport).toHaveBeenCalled();
  });

  it('连接后可开关 Deep Profile 与 Call Stacks', async () => {
    const onDeepProfiling = vi.fn();
    const onCallStacks = vi.fn();
    renderBar({ onDeepProfiling, onCallStacks });
    await userEvent.click(screen.getByLabelText('Deep Profile'));
    await userEvent.click(screen.getByLabelText('Call Stacks'));
    expect(onDeepProfiling).toHaveBeenCalledWith(true);
    expect(onCallStacks).toHaveBeenCalledWith(true);
  });

  it('未连接时 Deep Profile 与 Call Stacks 不可用', () => {
    renderBar({ connected: false, recording: false });
    expect(screen.getByLabelText('Deep Profile')).toBeDisabled();
    expect(screen.getByLabelText('Call Stacks')).toBeDisabled();
  });
});
