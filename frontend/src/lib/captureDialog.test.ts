import { describe, expect, it, vi, beforeEach } from 'vitest';

vi.mock('@wailsio/runtime', () => ({
  Dialogs: {
    OpenFile: vi.fn(),
    SaveFile: vi.fn(),
  },
}));

import { Dialogs } from '@wailsio/runtime';
import { captureDialogs } from './captureDialog';

describe('captureDialogs', () => {
  beforeEach(() => {

    vi.mocked(Dialogs.OpenFile).mockReset();
    vi.mocked(Dialogs.SaveFile).mockReset();
  });


  it('open 取消时返回空字符串', async () => {
    vi.mocked(Dialogs.OpenFile).mockResolvedValue('');
    await expect(captureDialogs.open()).resolves.toBe('');
  });

  it('open 用户取消对话框时不抛错', async () => {
    vi.mocked(Dialogs.OpenFile).mockRejectedValue(
      new Error('Invalid dialog call: Dialog.OpenFile failed, error getting selection: cancelled by user'),
    );
    await expect(captureDialogs.open()).resolves.toBe('');
  });


  it('open 其它错误仍然抛出', async () => {
    vi.mocked(Dialogs.OpenFile).mockRejectedValue(new Error('disk error'));
    await expect(captureDialogs.open()).rejects.toThrow('disk error');
  });

  it('open 多选结果取第一项，并支持 .upac 与 .data', async () => {
    vi.mocked(Dialogs.OpenFile).mockResolvedValue(['a.upac', 'b.data'] as unknown as string);
    await expect(captureDialogs.open()).resolves.toBe('a.upac');
    expect(Dialogs.OpenFile).toHaveBeenCalledWith(expect.objectContaining({
      Filters: [
        { DisplayName: 'UnityPerfAgent Capture (*.upac)', Pattern: '*.upac' },
        { DisplayName: 'Unity Profiler Data (*.data)', Pattern: '*.data' },
      ],
    }));
  });

  it('save 使用 .upac 文件名和过滤器', async () => {
    vi.mocked(Dialogs.SaveFile).mockResolvedValue('capture.upac');
    await expect(captureDialogs.save()).resolves.toBe('capture.upac');
    expect(Dialogs.SaveFile).toHaveBeenCalledWith(expect.objectContaining({
      Filename: expect.stringMatching(/^profiler-.*\.upac$/),
      Filters: [{ DisplayName: 'UnityPerfAgent Capture (*.upac)', Pattern: '*.upac' }],
    }));
  });
});
