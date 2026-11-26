// NOTE: this file intentionally uses \uXXXX escapes for CJK strings to stay
// ASCII-only (the file was repeatedly corrupted by editor encoding issues).
import type { ComponentProps } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ConnectBar } from './ConnectBar';
import type { Status } from '../types';

// \u65ad\u5f00\u8fde\u63a5 = disconnect,
// USB \u8fde\u63a5 = USB connect, \u5e27 = frames
// \u5e94\u7528\u5305\u540d = app package name
const BTN_DISCONNECT = '\u65ad\u5f00\u8fde\u63a5';
const BTN_USB = 'USB \u8fde\u63a5';
const LABEL_PACKAGE = '\u5e94\u7528\u5305\u540d';
const FRAMES_TEXT = '12,345 \u5e27';

const disconnected: Status = {
  connected: false,
  mode: 'profiler',
  device: { deviceName: '', deviceModel: '', os: '', unityVersion: '', appVersion: '' },
  totalFrames: 0,
  retained: 0,
  unitySupported: false,
};

const connected: Status = {
  connected: true,
  mode: 'profiler',
  device: { deviceName: 'Pixel 7', deviceModel: 'Google Pixel 7', os: 'Android 14', unityVersion: '2022.3.47f1', appVersion: '1.0.0' },
  totalFrames: 12345,
  retained: 12345,
  unitySupported: true,
};

const unsupported: Status = {
  connected: true,
  mode: 'profiler',
  device: { deviceName: 'AndroidPlayer', deviceModel: 'Device', os: 'Android 10', unityVersion: '2021.3.30f1', appVersion: '1.0.0' },
  totalFrames: 661,
  retained: 661,
  unitySupported: false,
};

const usbConnected: Status = {
  connected: true,
  mode: 'profiler',
  transport: 'adb',
  device: { deviceName: 'AndroidPlayer(USB:Mi MIX 2S)', deviceModel: 'Mi MIX 2S', os: 'Android 10', unityVersion: '2022.3.47f1', appVersion: '1.0.0' },
  totalFrames: 12,
  retained: 12,
  unitySupported: true,
};

const noop = () => {};
beforeEach(() => localStorage.clear());

function renderBar(override: Partial<ComponentProps<typeof ConnectBar>> = {}) {
  const props: ComponentProps<typeof ConnectBar> = {
    status: disconnected,
    mode: 'app', onModeChange: noop,
    busy: false,
    packageId: '',
    onPackageIdChange: noop,
    onDisconnect: noop,
    onUSB: noop,
    onClear: noop,
    ...override,
  };
  return render(<ConnectBar {...props} />);
}

describe('ConnectBar', () => {
  it('connects by phone IP in App mode', async () => {
    const onDirect = vi.fn();
    renderBar({ onDirect });
    const button = screen.getByRole('button', { name: 'IP 直连' });
    expect(button).toBeDisabled();
    await userEvent.type(screen.getByLabelText('手机 IP'), ' 192.168.0.42 ');
    await userEvent.click(button);
    expect(onDirect).toHaveBeenCalledWith('192.168.0.42');
  });

  it('shows package input and USB button when disconnected', () => {
    renderBar({ packageId: 'com.company.game' });
    expect(screen.getByLabelText(LABEL_PACKAGE)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: BTN_USB })).toBeEnabled();
    expect(screen.queryByRole('button', { name: '\u6f14\u793a\u6a21\u5f0f' })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('\u8bbe\u5907 IP')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('\u7aef\u53e3')).not.toBeInTheDocument();
  });

  it('disables USB connect when package id is empty', () => {
    renderBar({ packageId: '' });
    expect(screen.getByRole('button', { name: BTN_USB })).toBeDisabled();
  });

  it('shows device info and disconnect button when connected', async () => {
    const onDisconnect = vi.fn();
    renderBar({ status: connected, packageId: 'com.company.game', onDisconnect });
    expect(screen.getByText(/Pixel 7/)).toBeInTheDocument();
    expect(screen.getByText('Unity 2022.3.47f1')).toBeInTheDocument();
    expect(screen.getByText(FRAMES_TEXT)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: BTN_DISCONNECT }));
    expect(onDisconnect).toHaveBeenCalled();
  });

  it('usb button triggers callback', async () => {
    const onUSB = vi.fn();
    renderBar({ packageId: 'com.company.game', onUSB });
    await userEvent.click(screen.getByRole('button', { name: BTN_USB }));
    expect(onUSB).toHaveBeenCalled();
  });

  it('starts Mini Game autoconnect without an Android package ID', async () => {
    const onAutoconnect = vi.fn();
    renderBar({ mode: 'mini', onAutoconnect, autoconnectAddresses: ['192.168.0.42', '10.0.0.8'] });
    const button = screen.getByRole('button', { name: '开启小游戏监听' });
    expect(button).toHaveClass('minigame-listener-button');
    expect(button.nextElementSibling).toHaveTextContent('本机 IP 192.168.0.42 / 10.0.0.8');
    await userEvent.click(button);
    expect(onAutoconnect).toHaveBeenCalledOnce();
  });

  it.each([false, true])('offers a close button while the Mini Game port is listening (connected: %s)', async (isConnected) => {
    const onDisconnect = vi.fn();
    renderBar({
      mode: 'mini', status: { ...connected, connected: isConnected, reconnecting: !isConnected, transport: 'autoconnect' },
      onDisconnect,
    });
    const button = screen.getByRole('button', { name: '关闭小游戏监听' });
    expect(button).toHaveClass('minigame-listener-button');
    await userEvent.click(button);
    expect(onDisconnect).toHaveBeenCalledOnce();
  });

  it('keeps the address visible while waiting for the Mini Game', () => {
    renderBar({
      mode: 'mini', status: { ...usbConnected, connected: false, reconnecting: true, transport: 'autoconnect' },
      autoconnectAddresses: ['192.168.0.42'],
      packageId: 'com.company.game',
    });
    expect(screen.getByText(/等待小游戏连接/)).toBeInTheDocument();
    expect(screen.getByText('192.168.0.42')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: BTN_USB })).toBeNull();
  });

  it('opens Unity logs from the connection toolbar', async () => {
    const onUnityLogs = vi.fn();
    const onMemory = vi.fn();
    renderBar({ onUnityLogs, onMemory });
    const memory = screen.getByRole('button', { name: 'Memory Profile' });
    expect(screen.getByRole('button', { name: 'Unity 日志' }).nextElementSibling).toBe(memory);
    await userEvent.click(memory);
    expect(onMemory).toHaveBeenCalledOnce();
    await userEvent.click(screen.getByRole('button', { name: 'Unity 日志' }));
    expect(onUnityLogs).toHaveBeenCalledOnce();
  });

  it('package input triggers change callback', async () => {
    const onPackageIdChange = vi.fn();
    renderBar({ packageId: 'com.old', onPackageIdChange });
    await userEvent.clear(screen.getByLabelText(LABEL_PACKAGE));
    await userEvent.type(screen.getByLabelText(LABEL_PACKAGE), 'com.new');
    expect(onPackageIdChange).toHaveBeenCalled();
  });

  it('shows USB label when connected via adb forward', () => {
    renderBar({ status: usbConnected, packageId: 'com.company.game' });
    expect(screen.getByText(/USB/)).toBeInTheDocument();
    expect(screen.getByText('Unity 2022.3.47f1')).toBeInTheDocument();
  });

  it('shows unsupported unity version warning', () => {
    renderBar({ status: unsupported, packageId: 'com.company.game' });
    expect(screen.getByText(/AndroidPlayer/)).toBeInTheDocument();
    const version = screen.getByText('Unity 2021.3.30f1');
    expect(version).toHaveClass('unity-unsupported');
  });

  it('shows waiting state and disconnect while reconnecting', async () => {
    const onDisconnect = vi.fn();
    const reconnecting: Status = {
      ...usbConnected,
      connected: false,
      reconnecting: true,
    };
    renderBar({ status: reconnecting, packageId: 'com.company.game', onDisconnect });
    expect(screen.getByText(/等待游戏恢复/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: BTN_USB })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: BTN_DISCONNECT }));
    expect(onDisconnect).toHaveBeenCalled();
  });
});

it('uses the detected phone IP and accepts a manual address', async () => {
  const onDirect = vi.fn();
  renderBar({ onDirect, phones: { ip: '10.1.2.3', endpoint: '10.1.2.3:5555', devices: [], error: '' } });
  const input = screen.getByLabelText('手机 IP');
  expect(input).toHaveValue('10.1.2.3');
  await userEvent.clear(input);
  await userEvent.type(input, '10.1.2.7');
  await userEvent.click(screen.getByRole('button', { name: 'IP 直连' }));
  expect(onDirect).toHaveBeenCalledWith('10.1.2.7');
});

it('restores manual IP after reopening and only displays the active mode controls', async () => {
  const onModeChange = vi.fn();
  const app = renderBar({ onDirect: noop, onAutoconnect: noop, onModeChange });
  expect(screen.queryByRole('button', { name: '开启小游戏监听' })).toBeNull();
  await userEvent.type(screen.getByLabelText('手机 IP'), '10.1.2.3');
  await userEvent.click(screen.getByRole('button', { name: '小程序' }));
  expect(onModeChange).toHaveBeenCalledWith('mini');
  app.unmount();
  const mini = renderBar({ mode: 'mini', onAutoconnect: noop, onDirect: noop });
  expect(screen.queryByLabelText('手机 IP')).toBeNull();
  expect(screen.queryByLabelText(LABEL_PACKAGE)).toBeNull();
  mini.unmount();
  renderBar({ onDirect: noop });
  expect(screen.getByLabelText('手机 IP')).toHaveValue('10.1.2.3');
});
