import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { CpuLeftHeavy } from './CpuLeftHeavy';

describe('CpuLeftHeavy', () => {
  it('matches Timeline hover, pinned selection and viewport gestures', async () => {
    const roots = [
      { name: 'First', category: 'Scripts', calls: 2, totalMs: 55, selfMs: 55 },
      { name: 'Second', category: 'Scripts', calls: 1, totalMs: 45, selfMs: 45 },
    ];
    const load = vi.fn(async () => ({ firstFrame: 1, lastFrame: 1, frameCount: 1, threads: [
      { id: 'main', name: 'Main Thread', roots, functions: roots },
    ] }));
    render(<CpuLeftHeavy range={{ start: 0, end: 0 }} totalFrames={1} load={load} onSelectRange={vi.fn()} />);
    const graph = await screen.findByLabelText('按累计耗时排序的调用栈火焰图');
    expect(window.getComputedStyle(graph).cursor).toBe('default');
    vi.spyOn(graph, 'getBoundingClientRect').mockReturnValue({ left: 100, top: 20, width: 400 } as DOMRect);
    const stats = screen.getByTestId('heavy-flame-stats');
    const hover = (clientX: number) => fireEvent.pointerMove(graph, { clientX, clientY: 25 });
    const wheel = (deltaY: number) => fireEvent.wheel(graph, { deltaY, clientX: 400, clientY: 25, cancelable: true });
    const click = (clientX: number, clientY = 25) => {
      fireEvent.pointerDown(graph, { button: 0, clientX, clientY, pointerId: 1 });
      fireEvent.pointerUp(graph, { button: 0, clientX, clientY, pointerId: 1 });
    };

    hover(308);
    expect(screen.getByRole('tooltip')).toHaveTextContent('First55.00ms · 2 次 · 平均 27.50ms');
    click(308);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
    expect(screen.getByTestId('heavy-flame-pinned-tip')).toHaveTextContent('First');
    hover(400);
    expect(screen.getByRole('tooltip')).toHaveTextContent('Second');
    expect(stats).toHaveTextContent('First');
    fireEvent.pointerLeave(graph.parentElement!);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
    expect(screen.getByTestId('heavy-flame-pinned-tip')).toBeInTheDocument();
    click(308, 70);
    expect(screen.queryByTestId('heavy-flame-pinned-tip')).not.toBeInTheDocument();

    expect(wheel(-100)).toBe(false);
    hover(400);
    expect(screen.getByRole('tooltip')).toHaveTextContent('Second');
    hover(308);
    expect(screen.getByRole('tooltip')).toHaveTextContent('Second');

    fireEvent.pointerDown(graph, { button: 1, clientX: 250, clientY: 25, pointerId: 2 });
    expect(window.getComputedStyle(graph).cursor).toBe('grabbing');
    fireEvent.pointerMove(graph, { clientX: 350, clientY: 25, pointerId: 2 });
    fireEvent.pointerUp(graph, { button: 1, clientX: 350, clientY: 25, pointerId: 2 });
    expect(window.getComputedStyle(graph).cursor).toBe('default');
    hover(308);
    expect(screen.getByRole('tooltip')).toHaveTextContent('First');
    expect(screen.queryByTestId('heavy-flame-pinned-tip')).not.toBeInTheDocument();

    fireEvent.wheel(graph, { deltaY: 120, shiftKey: true, clientX: 308, clientY: 25, cancelable: true });
    expect(screen.getByRole('tooltip')).toHaveTextContent('Second');

    wheel(100);
    hover(308);
    expect(screen.getByRole('tooltip')).toHaveTextContent('First');
    expect(screen.queryByRole('button', { name: '恢复全图' })).not.toBeInTheDocument();

    fireEvent.doubleClick(graph, { clientX: 400, clientY: 25 });
    expect(screen.getByTestId('heavy-flame-pinned-tip')).toHaveTextContent('Second');
    expect(stats).toHaveTextContent('Second');
    fireEvent.click(screen.getByRole('button', { name: '恢复全图' }));
    hover(110);
    expect(screen.getByRole('tooltip')).toHaveTextContent('First');

    fireEvent.pointerDown(graph, { button: 2, altKey: true, clientX: 400, clientY: 25, pointerId: 3 });
    fireEvent.pointerMove(graph, { altKey: true, clientX: 464, clientY: 25, pointerId: 3 });
    fireEvent.pointerUp(graph, { button: 2, altKey: true, clientX: 464, clientY: 25, pointerId: 3 });
    expect(screen.getByRole('button', { name: '恢复全图' })).toBeInTheDocument();
    expect(stats).toHaveTextContent('Second');
    fireEvent.keyDown(graph, { key: 'a' });
    expect(screen.queryByRole('button', { name: '恢复全图' })).not.toBeInTheDocument();
    fireEvent.keyDown(graph, { key: 'f' });
    expect(screen.getByRole('button', { name: '恢复全图' })).toBeInTheDocument();
  });
});
