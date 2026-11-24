import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { StatusBar } from './StatusBar';
import type { Status } from '../types';

const disconnected: Status = {
  connected: false,
  mode: 'profiler',
  device: { deviceName: '', deviceModel: '', os: '', unityVersion: '', appVersion: '' },
  totalFrames: 0,
  retained: 0,
  unitySupported: false,
};

const live: Status = {
  connected: true,
  mode: 'profiler',
  transport: 'adb',
  device: { deviceName: 'Pixel', deviceModel: 'Pixel', os: 'Android 14', unityVersion: '2022.3.47f1', appVersion: '1.0' },
  totalFrames: 42,
  retained: 42,
  unitySupported: true,
  recording: true,
};

describe('StatusBar', () => {
  it('未连接时显示空闲', () => {
    render(<StatusBar status={disconnected} />);
    expect(screen.getByTestId('statusbar')).toBeInTheDocument();
    expect(screen.getByText('未连接')).toBeInTheDocument();
    expect(screen.getByText('空闲')).toBeInTheDocument();
  });

  it('连接采集时显示 USB / 采集中 / Unity 版本', () => {
    render(<StatusBar status={live} />);
    expect(screen.getByText('USB 已连接')).toBeInTheDocument();
    expect(screen.getByText('采集中')).toBeInTheDocument();
    expect(screen.getByText('42 帧')).toBeInTheDocument();
    expect(screen.getByText('Unity 2022.3.47f1')).toBeInTheDocument();
  });

  it('重连中显示等待游戏恢复', () => {
    render(<StatusBar status={{ ...live, connected: false, reconnecting: true }} />);
    expect(screen.getByText('等待游戏恢复')).toBeInTheDocument();
    expect(screen.getByText('等待恢复')).toBeInTheDocument();
  });

  it('busy 时显示处理中', () => {
    render(<StatusBar status={disconnected} busy />);
    expect(screen.getByText('处理中…')).toBeInTheDocument();
  });
});
