import { describe, expect, it } from 'vitest';
import { connectionLabel, frameCountLabel, recordingLabel } from './statusBar';
import type { Status } from '../types';

const base: Status = {
  connected: false,
  mode: 'profiler',
  device: { deviceName: '', deviceModel: '', os: '', unityVersion: '', appVersion: '' },
  totalFrames: 0,
  retained: 0,
  unitySupported: false,
};

describe('statusBar labels', () => {
  it('未连接为空闲', () => {
    expect(connectionLabel(base)).toBe('未连接');
    expect(recordingLabel(base)).toBe('空闲');
    expect(frameCountLabel(base)).toBe('0 帧');
  });

  it('USB 连接且采集中', () => {
    const st: Status = { ...base, connected: true, transport: 'adb', recording: true, retained: 12, totalFrames: 12 };
    expect(connectionLabel(st)).toBe('USB 已连接');
    expect(recordingLabel(st)).toBe('采集中');
    expect(frameCountLabel(st)).toBe('12 帧');
  });

  it('暂停时显示已暂停，保留帧少于累计帧时显示两者', () => {
    const st: Status = { ...base, connected: true, recording: false, retained: 100, totalFrames: 200 };
    expect(recordingLabel(st)).toBe('已暂停');
    expect(frameCountLabel(st)).toBe('100 / 200 帧');
  });

  it('自动重连时显示等待游戏恢复', () => {
    const st: Status = { ...base, reconnecting: true, transport: 'adb', retained: 12, totalFrames: 12 };
    expect(connectionLabel(st)).toBe('等待游戏恢复');
    expect(recordingLabel(st)).toBe('等待恢复');
  });
});
