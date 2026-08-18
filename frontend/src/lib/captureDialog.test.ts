import { describe, expect, it, vi, beforeEach } from 'vitest';

vi.mock('@wailsio/runtime', () => ({
  Dialogs: {
    OpenFile: vi.fn(),
  },
}));

import { Dialogs } from '@wailsio/runtime';
import { captureDialogs } from './captureDialog';

describe('captureDialogs', () => {
  beforeEach(() => {

    vi.mocked(Dialogs.OpenFile).mockReset();
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

  it('open 多选结果取第一项，并限制为 .data', async () => {
    vi.mocked(Dialogs.OpenFile).mockResolvedValue(['a.data', 'b.data'] as unknown as string);
    await expect(captureDialogs.open()).resolves.toBe('a.data');
    expect(Dialogs.OpenFile).toHaveBeenCalledWith(expect.objectContaining({
      Filters: [{ DisplayName: 'Unity Profiler Data (*.data)', Pattern: '*.data' }],
    }));
  });
});
