import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfilerDetail } from './ProfilerDetail';
import { emptyFrame } from '../lib/frame';

const frame = emptyFrame({
  cpuTimeMs: 16.4,
  cpuRenderingMs: 5,
  cpuScriptsMs: 4,
  gpuTimeMs: 11,
  renderMs: 6,
  usedMemMB: 512,
  reservedMB: 700,
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
  it('CPU Hierarchy 列出 Rendering / Scripts', () => {
    render(<ProfilerDetail moduleId="cpu" frame={frame} />);
    expect(screen.getByText('Rendering')).toBeInTheDocument();
    expect(screen.getByText('Scripts')).toBeInTheDocument();
    expect(screen.getByText('5.0ms')).toBeInTheDocument();
  });

  it('无选中帧显示提示', () => {
    render(<ProfilerDetail moduleId="cpu" frame={null} />);
    expect(screen.getByText('选择一帧以查看详情')).toBeInTheDocument();
  });

  it('Memory Detailed 页签提示不支持快照', async () => {
    render(<ProfilerDetail moduleId="memory" frame={frame} />);
    await userEvent.click(screen.getByRole('tab', { name: 'Detailed' }));
    expect(screen.getByText(/内存快照/)).toBeInTheDocument();
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
    expect(screen.getByText('34.3ms')).toBeInTheDocument();

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
});
