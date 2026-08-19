import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MenuBar } from './MenuBar';

function renderBar(over: Partial<Parameters<typeof MenuBar>[0]> = {}) {
  return render(
    <MenuBar
      onImport={vi.fn()}

      onClear={vi.fn()}
      onFrames={vi.fn()}
      onAnalysis={vi.fn()}
      onSettings={vi.fn()}
      {...over}
    />,
  );
}

describe('MenuBar', () => {
  it('展示文件 / 视图 / 诊断 / 设置', () => {
    renderBar();
    expect(screen.getByTestId('menubar')).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '文件' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '视图' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '诊断' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: 'Agent Log' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '设置' })).toBeInTheDocument();
  });

  it('文件菜单触发加载 / 清空', async () => {
    const onImport = vi.fn();
    const onClear = vi.fn();
    renderBar({ onImport, onClear });
    await userEvent.click(screen.getByRole('menuitem', { name: '文件' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '加载数据' }));
    expect(onImport).toHaveBeenCalled();


    await userEvent.click(screen.getByRole('menuitem', { name: '文件' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '清空数据' }));
    expect(onClear).toHaveBeenCalled();
  });


  it('视图菜单打开帧数据', async () => {
    const onFrames = vi.fn();
    renderBar({ onFrames });
    await userEvent.click(screen.getByRole('menuitem', { name: '视图' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '帧数据' }));
    expect(onFrames).toHaveBeenCalled();
  });

  it('视图菜单切换 Agent 面板', async () => {
    const onToggleAgent = vi.fn();
    renderBar({ onToggleAgent, agentCollapsed: true });
    await userEvent.click(screen.getByRole('menuitem', { name: '视图' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '展开 Agent 面板' }));
    expect(onToggleAgent).toHaveBeenCalled();
  });

  it('诊断、Agent Log 与设置直接触发对应操作', async () => {
    const onAnalysis = vi.fn();
    const onSettings = vi.fn();
    const onOpenAgentLog = vi.fn();
    renderBar({ onAnalysis, onSettings, onOpenAgentLog });
    await userEvent.click(screen.getByRole('menuitem', { name: '诊断' }));
    await userEvent.click(screen.getByRole('menuitem', { name: 'Agent Log' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '设置' }));
    expect(onAnalysis).toHaveBeenCalled();
    expect(onOpenAgentLog).toHaveBeenCalled();
    expect(onSettings).toHaveBeenCalled();
  });
});
