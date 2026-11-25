import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MenuBar } from './MenuBar';

function renderBar(over: Partial<Parameters<typeof MenuBar>[0]> = {}) {
  return render(
    <MenuBar
      onImport={vi.fn()}
      onExport={vi.fn()}
      onClear={vi.fn()}
      onFrames={vi.fn()}
      onAnalysis={vi.fn()}
      {...over}
    />,
  );
}

describe('MenuBar', () => {
  it('offers native Unity data export', async () => {
    const onExportUnityData = vi.fn();
    renderBar({ onExportUnityData });
    await userEvent.click(screen.getByRole('menuitem', { name: '文件' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '导出 Unity .data' }));
    expect(onExportUnityData).toHaveBeenCalledOnce();
  });
  it('展示文件、视图和诊断入口', () => {
    renderBar();
    expect(screen.getByTestId('menubar')).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '文件' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '视图' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '诊断' })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: 'Agent Log' })).not.toBeInTheDocument();
  });

  it('文件菜单触发加载 / 保存 / 清空', async () => {
    const onImport = vi.fn();
    const onExport = vi.fn();
    const onClear = vi.fn();
    renderBar({ onImport, onExport, onClear });
    await userEvent.click(screen.getByRole('menuitem', { name: '文件' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '加载数据' }));
    expect(onImport).toHaveBeenCalled();

    await userEvent.click(screen.getByRole('menuitem', { name: '文件' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '保存数据 (.upac)' }));
    expect(onExport).toHaveBeenCalled();

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

  it('视图菜单只提供帧数据入口', async () => {
    renderBar();
    await userEvent.click(screen.getByRole('menuitem', { name: '视图' }));
    expect(screen.getAllByRole('menuitem', { name: '帧数据' })).toHaveLength(1);
    expect(screen.queryByRole('menuitem', { name: /Agent 面板/ })).not.toBeInTheDocument();
  });

  it('诊断入口触发对应操作', async () => {
    const onAnalysis = vi.fn();
    renderBar({ onAnalysis });
    await userEvent.click(screen.getByRole('menuitem', { name: '诊断' }));
    expect(onAnalysis).toHaveBeenCalledOnce();
  });
});

it('shows update progress and the result in the save-style modal', async () => {
  let finish!: (message: string) => void;
  renderBar({ checkUpdate: () => new Promise(resolve => { finish = resolve; }) });
  await userEvent.click(screen.getByRole('menuitem', { name: '检查更新' }));
  expect(screen.getByRole('progressbar', { name: '检查更新进度' })).toBeInTheDocument();
  finish('当前已是最新版本 v0.2.0');
  const modal = await screen.findByRole('dialog', { name: '检查更新完成' });
  expect(modal).toHaveTextContent('当前已是最新版本 v0.2.0');
  await userEvent.click(screen.getByRole('button', { name: '确定' }));
  expect(screen.queryByRole('dialog')).toBeNull();
});
