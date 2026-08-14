import { describe, expect, it, vi, beforeEach } from 'vitest';

vi.mock('@wailsio/runtime', () => ({
  Dialogs: {
    SaveFile: vi.fn(),
    OpenFile: vi.fn(),
  },
}));

import { Dialogs } from '@wailsio/runtime';
import { captureDialogs } from './captureDialog';

describe('captureDialogs', () => {
  beforeEach(() => {
    vi.mocked(Dialogs.SaveFile).mockReset();
    vi.mocked(Dialogs.OpenFile).mockReset();
  });

  it('save 返回用户选择的路径', async () => {
    vi.mocked(Dialogs.SaveFile).mockResolvedValue('D:\\cap.upac');
    await expect(captureDialogs.save()).resolves.toBe('D:\\cap.upac');
    expect(Dialogs.SaveFile).toHaveBeenCalled();
  });

  it('open 取消时返回空字符串', async () => {
    vi.mocked(Dialogs.OpenFile).mockResolvedValue('');
    await expect(captureDialogs.open()).resolves.toBe('');
  });

  it('open 多选结果取第一项', async () => {
    vi.mocked(Dialogs.OpenFile).mockResolvedValue(['a.upac', 'b.upac'] as unknown as string);
    await expect(captureDialogs.open()).resolves.toBe('a.upac');
  });
});
