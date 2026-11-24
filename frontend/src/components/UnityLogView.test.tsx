import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { UnityLogView } from './UnityLogView';

it('shows the native log wait state for a Mini Game', () => {
  render(<UnityLogView miniGame fetchLogs={() => new Promise(() => {})} />);
  expect(screen.getByText('等待小游戏输出 Unity 日志。')).toBeInTheDocument();
});

it('streams new Unity entries without replacing existing logs', async () => {
  vi.useFakeTimers();
  const fetchLogs = vi.fn()
    .mockResolvedValueOnce([{ id: 1, receivedAt: 1000, level: 'Warning', text: 'frame warning' }])
    .mockResolvedValueOnce([{ id: 2, receivedAt: 2000, level: 'Exception', text: 'Player.Update()' }]);
  const view = render(<UnityLogView fetchLogs={fetchLogs} />);
  try {
    await act(async () => {});
    expect(screen.getByText('frame warning')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /frame warning/ }));
    await act(async () => { await vi.advanceTimersByTimeAsync(500); });
    expect(fetchLogs).toHaveBeenNthCalledWith(2, 1);
    expect(screen.getByText('Exception')).toBeInTheDocument();
    expect(screen.getByText('Player.Update()')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /frame warning/, pressed: true })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: '日志详情' })).toHaveTextContent('frame warning');
  } finally {
    view.unmount();
    vi.useRealTimers();
  }
});

it('combines level filters with case-insensitive message and stack searches', async () => {
  const fetchLogs = vi.fn().mockResolvedValue([]).mockResolvedValueOnce([
    { id: 1, receivedAt: 1000, level: 'Log', text: 'Player ready' },
    { id: 2, receivedAt: 2000, level: 'Warning', text: 'Player slow' },
    { id: 3, receivedAt: 3000, level: 'Error', text: 'Player failed' },
    { id: 4, receivedAt: 4000, level: 'Assert', text: 'Invalid state' },
    { id: 5, receivedAt: 5000, level: 'Exception', text: 'Crash\nPlayer.Update()' },
  ]);
  render(<UnityLogView fetchLogs={fetchLogs} />);
  const entries = ['Player ready', 'Player slow', 'Player failed', 'Invalid state', 'Crash'];
  await screen.findByText(entries[0]);
  for (const entry of entries) expect(screen.getByText(entry)).toBeInTheDocument();

  const log = screen.getByRole('button', { name: 'Log', pressed: true });
  const warning = screen.getByRole('button', { name: 'Warning', pressed: true });
  const error = screen.getByRole('button', { name: 'Error', pressed: true });
  fireEvent.click(log);
  expect(log).toHaveAttribute('aria-pressed', 'false');
  expect(screen.queryByText(entries[0])).not.toBeInTheDocument();
  fireEvent.click(warning);
  expect(screen.queryByText(entries[1])).not.toBeInTheDocument();
  for (const entry of entries.slice(2)) expect(screen.getByText(entry)).toBeInTheDocument();

  const search = screen.getByRole('searchbox', { name: '搜索日志' });
  fireEvent.change(search, { target: { value: ' PLAYER.UPDATE() ' } });
  expect(screen.getByText(entries[4])).toBeInTheDocument();
  expect(screen.queryByText(entries[2])).not.toBeInTheDocument();
  expect(screen.queryByText(entries[3])).not.toBeInTheDocument();
  fireEvent.click(error);
  expect(screen.getByText('没有符合当前筛选条件的日志。')).toBeInTheDocument();
  expect(screen.queryByText(entries[4])).not.toBeInTheDocument();
  fireEvent.click(error);
  fireEvent.change(search, { target: { value: '' } });
  for (const entry of entries.slice(2)) expect(screen.getByText(entry)).toBeInTheDocument();
  expect(screen.queryByText(entries[0])).not.toBeInTheDocument();
  expect(screen.queryByText(entries[1])).not.toBeInTheDocument();
});

it('applies active filters to incoming logs and retains hidden entries', async () => {
  vi.useFakeTimers();
  const fetchLogs = vi.fn()
    .mockResolvedValueOnce([])
    .mockResolvedValueOnce([
      { id: 1, receivedAt: 1000, level: 'Warning', text: 'matching warning' },
      { id: 2, receivedAt: 2000, level: 'Log', text: 'matching log' },
      { id: 3, receivedAt: 3000, level: 'Log', text: 'other entry' },
    ]);
  const view = render(<UnityLogView fetchLogs={fetchLogs} />);
  try {
    await act(async () => {});
    const warning = screen.getByRole('button', { name: 'Warning' });
    const search = screen.getByRole('searchbox');
    fireEvent.click(warning);
    fireEvent.change(search, { target: { value: 'matching' } });
    await act(async () => { await vi.advanceTimersByTimeAsync(500); });
    expect(screen.getByText('matching log')).toBeInTheDocument();
    expect(screen.queryByText('matching warning')).not.toBeInTheDocument();
    expect(screen.queryByText('other entry')).not.toBeInTheDocument();
    fireEvent.click(warning);
    expect(screen.getByText('matching warning')).toBeInTheDocument();
    fireEvent.change(search, { target: { value: '' } });
    expect(screen.getByText('other entry')).toBeInTheDocument();
  } finally {
    view.unmount();
    vi.useRealTimers();
  }
});

it('limits previews to the chosen line count and shows the complete selected log below', async () => {
  const trace = 'Crash\r\nPlayer.Update()\r\nGame.Run()';
  const fetchLogs = vi.fn().mockResolvedValue([]).mockResolvedValueOnce([
    { id: 1, receivedAt: 1000, level: 'Exception', text: trace },
    { id: 2, receivedAt: 2000, level: 'Log', text: 'Player ready' },
  ]);
  render(<UnityLogView fetchLogs={fetchLogs} />);
  const crash = await screen.findByRole('button', { name: /Crash/ });
  const detail = screen.getByRole('region', { name: '日志详情' });
  const lineCount = screen.getByRole('combobox', { name: '每条日志预览行数' });
  expect(lineCount).toHaveValue('1');
  expect(within(lineCount).getAllByRole('option')).toHaveLength(10);
  expect(within(crash).getByText('Crash')).toBeInTheDocument();
  expect(within(crash).queryByText(/Player\.Update/)).not.toBeInTheDocument();
  fireEvent.click(crash);
  expect(crash).toHaveAttribute('aria-pressed', 'true');
  expect(detail.querySelector('pre')?.textContent).toBe(trace);

  fireEvent.change(lineCount, { target: { value: '2' } });
  expect(within(crash).getByText(/Player\.Update/)).toBeInTheDocument();
  expect(within(crash).queryByText(/Game\.Run/)).not.toBeInTheDocument();
  fireEvent.change(lineCount, { target: { value: '10' } });
  expect(within(crash).getByText(/Game\.Run/)).toBeInTheDocument();

  const timestamp = screen.getByRole('button', { name: '时间戳', pressed: true });
  expect(crash.querySelector('time')).toBeInTheDocument();
  fireEvent.click(timestamp);
  expect(crash.querySelector('time')).not.toBeInTheDocument();
  expect(detail.querySelector('pre')?.textContent).toBe(trace);
  fireEvent.click(timestamp);
  expect(crash.querySelector('time')).toBeInTheDocument();

  const ready = screen.getByRole('button', { name: /Player ready/ });
  fireEvent.click(ready);
  expect(ready).toHaveAttribute('aria-pressed', 'true');
  expect(crash).toHaveAttribute('aria-pressed', 'false');
  expect(detail.querySelector('pre')?.textContent).toBe('Player ready');
  fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'Game.Run' } });
  expect(detail.querySelector('pre')).not.toBeInTheDocument();
});
