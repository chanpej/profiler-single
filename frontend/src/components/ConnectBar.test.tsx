// NOTE: this file intentionally uses \uXXXX escapes for CJK strings to stay
// ASCII-only (the file was repeatedly corrupted by editor encoding issues).
import type { ComponentProps } from 'react';
import { describe, expect, it, vi } from 'vitest';
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

function renderBar(override: Partial<ComponentProps<typeof ConnectBar>> = {}) {
  const props: ComponentProps<typeof ConnectBar> = {
    status: disconnected,
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
