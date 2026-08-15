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
  it('去掉 Error 前缀', () => {
    expect(formatErrorMessage(new Error('boom'))).toBe('boom');
    expect(formatErrorMessage('Error: nope')).toBe('nope');
  });
});
