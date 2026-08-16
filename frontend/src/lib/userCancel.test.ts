import { describe, expect, it } from 'vitest';
import { formatErrorMessage, isUserCancelled } from './userCancel';

describe('isUserCancelled', () => {
  it('识别 Wails 文件对话框取消', () => {
    expect(isUserCancelled(
      new Error('Invalid dialog call: Dialog.OpenFile failed, error getting selection: cancelled by user'),
    )).toBe(true);
    expect(isUserCancelled('Dialog.SaveFile failed, canceled by user')).toBe(true);
  });

  it('其它错误不算取消', () => {
    expect(isUserCancelled(new Error('disk full'))).toBe(false);
    expect(isUserCancelled('operation cancelled by timeout')).toBe(false);
  });
});

describe('formatErrorMessage', () => {
  it('去掉 Error 前缀，保留解析失败原因', () => {
    expect(formatErrorMessage(new Error('boom'))).toBe('boom');
    expect(formatErrorMessage('Error: nope')).toBe('nope');
    expect(formatErrorMessage('Error: 解析存档失败：缺少 Profiler 数据流头（PD3U），无法解析。请重新采集并保存完整数据')).toBe(
      '解析存档失败：缺少 Profiler 数据流头（PD3U），无法解析。请重新采集并保存完整数据',
    );
  });
});
