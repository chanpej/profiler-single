import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { ScreenFilmstrip } from './ScreenFilmstrip';

it('uses the viewport to fetch screenshots and selects the associated frame', async () => {
  const shot = { id: '100', frame: 12, image: 'aW1hZ2U=', deltaMs: 42, captureMs: 180 };
  const sameImage = { ...shot, id: '200', frame: 16 };
  const fetchStrip = vi.fn(async () => ({ enabled: true, available: true, error: '', thumbnails: [shot, sameImage], selected: shot }));
  const onSelect = vi.fn();
  render(<ScreenFilmstrip fetchStrip={fetchStrip} setCapture={vi.fn()} connected start={10} count={10} selected={12} onSelect={onSelect} />);
  fireEvent.click(screen.getByRole('button', { name: '画面轨道' }));
  const thumbnail = await screen.findByRole('button', { name: '跳到采集帧 13' });
  expect(thumbnail).toHaveClass('is-selected');
  expect(screen.getByRole('button', { name: '跳到采集帧 17' })).not.toHaveClass('is-selected');
  await waitFor(() => expect(fetchStrip).toHaveBeenCalledWith(10, 10, 12));
  fireEvent.click(thumbnail);
  expect(onSelect).toHaveBeenCalledWith(12);
  fireEvent.click(screen.getByRole('button', { name: '预览选中帧' }));
  expect(screen.getByAltText('选中帧附近的手机画面')).toHaveAttribute('src', 'data:image/jpeg;base64,aW1hZ2U=');
  expect(screen.getByText(/估算偏移 \+42 ms/)).toBeInTheDocument();
});

it('prepares the wireless address before connecting and starts capture on that endpoint', async () => {
  const startWireless = vi.fn(async () => {});
  const enableWireless = vi.fn(async () => '192.168.0.7:5555');
  const props = { startWireless, enableWireless, setCapture: vi.fn(), start: 0, count: 0, selected: -1,
    onSelect: vi.fn(), fetchStrip: async () => ({ enabled: false, available: false, error: '', thumbnails: [], selected: null }) };
  const view = render(<ScreenFilmstrip {...props} connected={false} />);
  fireEvent.click(screen.getByLabelText('无线截图'));
  fireEvent.click(screen.getByRole('button', { name: '通过 USB 开启无线 ADB' }));
  await waitFor(() => expect(screen.getByLabelText('无线 ADB 地址')).toHaveValue('192.168.0.7:5555'));
  view.rerender(<ScreenFilmstrip {...props} connected />);
  fireEvent.click(screen.getByRole('button', { name: '采集截图' }));
  await waitFor(() => expect(startWireless).toHaveBeenCalledWith('192.168.0.7:5555'));
  expect(props.setCapture).not.toHaveBeenCalled();
});

it('places memory capture directly after the screenshot button and enables it for a connected Mini Game', () => {
  const props = { miniGame:true, fetchStrip:()=>new Promise<never>(()=>{}), setCapture:vi.fn(), start:0, count:0, selected:-1, onSelect:vi.fn() };
  const view = render(<ScreenFilmstrip {...props} connected={false} />);
  const button = screen.getByRole('button', { name:'Memory Profiler Capture' });
  expect(screen.getByRole('button', { name:'采集截图' }).nextElementSibling).toBe(button);
  expect(button).toBeDisabled();
  view.rerender(<ScreenFilmstrip {...props} connected />);
  expect(button).toBeEnabled();
});
