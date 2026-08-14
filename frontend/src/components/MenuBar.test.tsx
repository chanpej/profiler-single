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
    expect(screen.getByRole('menuitem', { name: '设置' })).toBeInTheDocument();
  });

  it('文件菜单触发加载 / 导出 / 清空', async () => {
    const onImport = vi.fn();
    const onExport = vi.fn();
    const onClear = vi.fn();
    renderBar({ onImport, onExport, onClear });
    await userEvent.click(screen.getByRole('menuitem', { name: '文件' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '加载数据' }));
    expect(onImport).toHaveBeenCalled();

    await userEvent.click(screen.getByRole('menuitem', { name: '文件' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '导出数据' }));
    expect(onExport).toHaveBeenCalled();

    await userEvent.click(screen.getByRole('menuitem', { name: '文件' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '清空数据' }));
    expect(onClear).toHaveBeenCalled();
  });

  it('无数据时禁用导出', async () => {
    renderBar({ canSave: false });
    await userEvent.click(screen.getByRole('menuitem', { name: '文件' }));
    expect(screen.getByRole('menuitem', { name: '导出数据' })).toBeDisabled();
  });

  it('视图菜单打开帧数据', async () => {
    const onFrames = vi.fn();
    renderBar({ onFrames });
    await userEvent.click(screen.getByRole('menuitem', { name: '视图' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '帧数据' }));
    expect(onFrames).toHaveBeenCalled();
  });

  it('诊断与设置直接打开对应面板', async () => {
    const onAnalysis = vi.fn();
    const onSettings = vi.fn();
    renderBar({ onAnalysis, onSettings });
    await userEvent.click(screen.getByRole('menuitem', { name: '诊断' }));
    await userEvent.click(screen.getByRole('menuitem', { name: '设置' }));
    expect(onAnalysis).toHaveBeenCalled();
    expect(onSettings).toHaveBeenCalled();
  });
});
