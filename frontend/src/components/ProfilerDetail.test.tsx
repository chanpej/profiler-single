import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfilerDetail } from './ProfilerDetail';
import { emptyFrame } from '../lib/frame';
import { DEFAULT_CALLSTACK_RATIO, DEFAULT_OVERVIEW_WIDTH } from '../lib/hierarchyLayout';

const frame = emptyFrame({
  cpuTimeMs: 16.4,
  cpuRenderingMs: 5,
  cpuScriptsMs: 4,
  gpuTimeMs: 11,
  renderMs: 6,
  usedMemMB: 512,
  reservedMB: 700,
  committedMemMB: 900,
  textureMemMB: 30,
  meshMemMB: 3,
  materialCount: 1003,
  objectCount: 4310,
  textureCount: 594,
  meshCount: 166,
  gameObjectCount: 4310,
  sceneObjectCount: 15974,
  gcUsedMB: 180,
  gcReservedMB: 240,
  gfxUsedMB: 160,
  drawCalls: 321,
  setPassCalls: 45,
  batches: 280,
  triangles: 1200000,
  playingSources: 9,
  audioVoices: 14,
  audioUsedMB: 8.5,
  activeRigidbodies: 42,
  colliders: 128,
  cpuPhysicsMs: 1.8,
  uiBatches: 24,
  uiVertices: 4800,
  cpuUiMs: 1.1,
  videoUsedMB: 0.5,
});

describe('ProfilerDetail', () => {
  afterEach(() => localStorage.clear());
  it('CPU Hierarchy 列出 Rendering / Scripts', () => {
    render(<ProfilerDetail moduleId="cpu" frame={frame} />);
    expect(screen.getByText('Rendering')).toBeInTheDocument();
    expect(screen.getByText('Scripts')).toBeInTheDocument();
    expect(screen.getByText('5.00ms')).toBeInTheDocument();
  });

  it('无选中帧显示提示', () => {
    render(<ProfilerDetail moduleId="cpu" frame={null} />);
    expect(screen.getByText('选择一帧以查看详情')).toBeInTheDocument();
  });

  it('Memory Simple 复刻 Editor 的 committed、breakdown 与对象统计结构', () => {
    render(<ProfilerDetail moduleId="memory" frame={frame} />);
    expect(screen.getByTestId('memory-simple')).toBeInTheDocument();
    expect(screen.getByText('Total Committed Memory')).toBeInTheDocument();
    expect(screen.getByText('Tracked Memory')).toBeInTheDocument();
    expect(screen.getAllByText('Untracked Memory')).toHaveLength(2);
    expect(screen.getByText('Total Memory Breakdown')).toBeInTheDocument();
    expect(screen.getByText('Graphics & Graphics Driver')).toBeInTheDocument();
    expect(screen.getByText('Object Statistics')).toBeInTheDocument();
    expect(screen.getByText('Textures')).toBeInTheDocument();
    expect(screen.getByText('Scene Objects')).toBeInTheDocument();
  });

  it('Memory Detailed 页签提示不支持快照', async () => {
    render(<ProfilerDetail moduleId="memory" frame={frame} />);
    await userEvent.click(screen.getByRole('tab', { name: 'Detailed' }));
    expect(screen.getByText(/内存快照/)).toBeInTheDocument();
  });

  it('Memory 掩码将未采集计数器显示为 —，不显示为零', () => {
    const partial = emptyFrame({ memoryCounterMask: 1 << 6, audioUsedMB: 3.5 });
    render(<ProfilerDetail moduleId="memory" frame={partial} />);
    const simple = screen.getByTestId('memory-simple');
    expect(simple).toHaveTextContent('Audio');
    expect(simple).toHaveTextContent('3.5 MB');
    expect(simple).toHaveTextContent(/未提供 App Committed Memory/);
  });

  it('Rendering / Audio / Physics / UI / GPU 计数器', () => {
    const { rerender } = render(<ProfilerDetail moduleId="rendering" frame={frame} />);
    expect(screen.getByText('SetPass Calls')).toBeInTheDocument();
    expect(screen.getByText('321')).toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="audio" frame={frame} />);
    expect(screen.getByText('Playing Sources')).toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="physics" frame={frame} />);
    expect(screen.getByText('Active Rigidbodies')).toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="ui" frame={frame} />);
    expect(screen.getByText('UI Batches')).toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="gpu" frame={frame} />);
    expect(screen.getByText('GPU Time')).toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="video" frame={frame} />);
    expect(screen.getByText('Video Memory')).toBeInTheDocument();
  });

  it('分类缺失帧：CPU Others 显示总耗时，Rendering 显示 Frame Time', () => {
    const coarse = emptyFrame({ fps: 59.9, cpuTimeMs: 34.3, usedMemMB: 700 });
    const { rerender } = render(<ProfilerDetail moduleId="cpu" frame={coarse} />);
    expect(screen.getByText('Others')).toBeInTheDocument();
    expect(screen.getByText('34.30ms')).toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="rendering" frame={coarse} />);
    expect(screen.getByText('Frame Time')).toBeInTheDocument();
  });

  it('有采样树时 Hierarchy 展示方法名，Call Stacks 显示调用栈', async () => {
    const tree = [{
      name: 'Main Thread',
      category: 'Internal',
      totalMs: 16,
      selfMs: 1,
      calls: 1,
      gcAllocKB: 0,
      children: [{
        name: 'GC.Alloc',
        category: 'GC',
        totalMs: 0.1,
        selfMs: 0.1,
        calls: 1,
        gcAllocKB: 4,
        callstack: ['GC.Alloc', 'PlayerController.Update ()'],
      }],
    }];
    render(<ProfilerDetail moduleId="cpu" frame={frame} hierarchy={tree} callStacks />);
    expect(screen.getByTestId('cpu-hierarchy')).toBeInTheDocument();
    expect(screen.getByText('Main Thread')).toBeInTheDocument();
    const headers = [...screen.getByTestId('hierarchy-table').querySelectorAll('th')]
      .map((cell) => cell.textContent);
    expect(headers).toEqual(['Overview', 'Total', 'Self', 'Calls', 'GC Alloc', 'Time ms', 'Self ms']);
    expect(screen.getByTestId('hierarchy-table')).toHaveTextContent('100.0%');
    expect(screen.getByTestId('hierarchy-table')).toHaveTextContent('6.3%');
    expect(screen.getByTestId('hierarchy-table')).toHaveTextContent('16.00ms');
    expect(screen.getByTestId('hierarchy-table')).toHaveTextContent('1.00ms');
    await userEvent.click(screen.getByText('GC.Alloc'));
    expect(screen.getByTestId('callstack-pane')).toHaveTextContent('PlayerController.Update');
  });

  // 真机上 GC.Alloc 在 5~7 层深，默认只展开两层，用户勾了 Call Stacks 却什么都看不到。
  it('勾选 Call Stacks 后自动展开深处的分配采样', async () => {
    const leaf = {
      name: 'GC.Alloc', category: 'GC', totalMs: 0.1, selfMs: 0.1, calls: 1, gcAllocKB: 4,
      callstack: ['0x727ff3a8c4', 'UIPanel.FillDrawCall()'],
    };
    const names = ['Main Thread', 'PlayerLoop', 'PreLateUpdate', 'UIPanel.LateUpdate', 'FillDrawCall'];
    const deep = [names.reduceRight<Record<string, unknown>>((child, name) => ({
      name, category: 'Internal', totalMs: 8, selfMs: 0, calls: 1, gcAllocKB: 4, children: [child],
    }), leaf)] as never;

    const { rerender } = render(<ProfilerDetail moduleId="cpu" frame={frame} hierarchy={deep} />);
    expect(screen.queryByText('GC.Alloc')).not.toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="cpu" frame={frame} hierarchy={deep} callStacks />);
    expect(screen.getByText('GC.Alloc')).toBeInTheDocument();
    await userEvent.click(screen.getByText('GC.Alloc'));
    expect(screen.getByTestId('callstack-pane')).toHaveTextContent('UIPanel.FillDrawCall');
  });

  it('本帧没有带调用栈的采样时说明原因', () => {
    const tree = [{
      name: 'Main Thread', category: 'Internal', totalMs: 16, selfMs: 16, calls: 1, gcAllocKB: 0,
    }];
    render(<ProfilerDetail moduleId="cpu" frame={frame} hierarchy={tree} callStacks />);
    expect(screen.getByTestId('callstack-pane')).toHaveTextContent('没有带调用栈的分配采样');
  });

  it('收起后即使 hierarchy 被刷新为新对象也保持收起', async () => {
    const tree = [{
      name: 'Main Thread',
      category: 'Internal',
      totalMs: 16,
      selfMs: 1,
      calls: 1,
      gcAllocKB: 0,
      children: [{
        name: 'BehaviourUpdate',
        category: 'Scripts',
        totalMs: 8,
        selfMs: 8,
        calls: 1,
        gcAllocKB: 0,
      }],
    }];
    const { rerender } = render(<ProfilerDetail moduleId="cpu" frame={frame} hierarchy={tree} />);
    expect(screen.getByText('BehaviourUpdate')).toBeInTheDocument();
    await userEvent.click(screen.getByLabelText('折叠'));
    expect(screen.queryByText('BehaviourUpdate')).not.toBeInTheDocument();

    rerender(<ProfilerDetail moduleId="cpu" frame={frame} hierarchy={[{ ...tree[0], children: [...tree[0].children!] }]} />);
    expect(screen.queryByText('BehaviourUpdate')).not.toBeInTheDocument();
  });

  it('Hierarchy 表头固定在独立滚动容器内，函数名截断并带完整名称气泡', () => {
    const tree = [{
      name: 'VeryLong.PlayerController.LateUpdate.FillDrawCall.Something',
      category: 'Scripts',
      totalMs: 16,
      selfMs: 16,
      calls: 1,
      gcAllocKB: 0,
    }];
    render(<ProfilerDetail moduleId="cpu" frame={frame} hierarchy={tree} />);
    expect(screen.getByTestId('hierarchy-scroll')).toBeInTheDocument();
    const name = screen.getByText(tree[0].name);
    expect(name).toHaveClass('hierarchy-name');
    expect(name).toHaveAttribute('title', tree[0].name);
    expect(screen.getByTestId('cpu-hierarchy').style.getPropertyValue('--overview-w')).toBe(`${DEFAULT_OVERVIEW_WIDTH}px`);
    const cols = screen.getByTestId('hierarchy-table').querySelectorAll('col');
    expect(cols).toHaveLength(7);
    expect(cols[0]).toHaveClass('col-overview');
    for (let i = 1; i < cols.length; i++) {
      expect(Number.parseInt(cols[i].style.width, 10)).toBeGreaterThan(0);
    }
  });

  it('拖拽 Overview 列宽，展开树后列宽不变', async () => {
    const tree = [{
      name: 'Main Thread',
      category: 'Internal',
      totalMs: 16,
      selfMs: 1,
      calls: 1,
      gcAllocKB: 0,
      children: [{
        name: 'BehaviourUpdate',
        category: 'Scripts',
        totalMs: 8,
        selfMs: 8,
        calls: 1,
        gcAllocKB: 0,
      }],
    }];
    render(<ProfilerDetail moduleId="cpu" frame={frame} hierarchy={tree} />);
    const handle = screen.getByTestId('overview-resize');
    fireEvent.pointerDown(handle, { clientX: 100, pointerId: 1 });
    fireEvent.pointerMove(handle, { clientX: 180, pointerId: 1 });
    fireEvent.pointerUp(handle, { clientX: 180, pointerId: 1 });
    const afterDrag = screen.getByTestId('cpu-hierarchy').style.getPropertyValue('--overview-w');
    expect(afterDrag).toBe('400px');

    await userEvent.click(screen.getByLabelText('折叠'));
    expect(screen.queryByText('BehaviourUpdate')).not.toBeInTheDocument();
    await userEvent.click(screen.getByLabelText('展开'));
    expect(screen.getByText('BehaviourUpdate')).toBeInTheDocument();
    expect(screen.getByTestId('cpu-hierarchy').style.getPropertyValue('--overview-w')).toBe(afterDrag);
  });

  it('Call Stacks 独立分栏可拖拽，复制写入剪贴板', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
    const tree = [{
      name: 'Main Thread',
      category: 'Internal',
      totalMs: 16,
      selfMs: 1,
      calls: 1,
      gcAllocKB: 0,
      children: [{
        name: 'GC.Alloc',
        category: 'GC',
        totalMs: 0.1,
        selfMs: 0.1,
        calls: 1,
        gcAllocKB: 4,
        callstack: ['GC.Alloc', 'PlayerController.Update ()'],
      }],
    }];
    render(<ProfilerDetail moduleId="cpu" frame={frame} hierarchy={tree} callStacks />);
    const copyBtn = screen.getByRole('button', { name: '复制' });
    expect(copyBtn).toBeDisabled();

    const split = screen.getByTestId('callstack-resize');
    fireEvent.pointerDown(split, { clientX: 700, pointerId: 1 });
    fireEvent.pointerMove(split, { clientX: 600, pointerId: 1 });
    fireEvent.pointerUp(split, { clientX: 600, pointerId: 1 });
    const pane = screen.getByTestId('callstack-pane');
    expect(pane.style.width).not.toBe(`${DEFAULT_CALLSTACK_RATIO * 100}%`);

    await userEvent.click(screen.getByText('GC.Alloc'));
    expect(copyBtn).toBeEnabled();
    await userEvent.click(copyBtn);
    expect(writeText).toHaveBeenCalledWith('GC.Alloc\nPlayerController.Update ()');
  });

  it('Timeline 展示采样泳道，单击选中', async () => {
    const tree = [{
      name: 'Main Thread',
      category: 'Internal',
      totalMs: 10,
      selfMs: 1,
      calls: 1,
      gcAllocKB: 0,
      startMs: 0,
      children: [{
        name: 'BehaviourUpdate',
        category: 'Scripts',
        totalMs: 3,
        selfMs: 3,
        calls: 1,
        gcAllocKB: 0,
        startMs: 2,
      }],
    }];
    render(<ProfilerDetail moduleId="cpu" frame={frame} hierarchy={tree} />);
    await userEvent.click(screen.getByRole('tab', { name: 'Timeline' }));
    expect(screen.getByTestId('cpu-timeline')).toBeInTheDocument();
    expect(screen.getByTestId('cpu-timeline-threads')).toHaveTextContent('Main Thread');
  });
});
