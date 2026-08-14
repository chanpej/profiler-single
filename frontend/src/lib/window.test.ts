import { describe, expect, it, vi } from 'vitest';
import { createWindowControls } from './window';

describe('createWindowControls', () => {
  it('无窗口时提供空操作', async () => {
    const win = createWindowControls();
    await expect(win.minimise()).resolves.toBeUndefined();
    await expect(win.toggleMaximise()).resolves.toBeUndefined();
    await expect(win.close()).resolves.toBeUndefined();
    await expect(win.isMaximised()).resolves.toBe(false);
  });

  it('转发到 Wails 窗口', async () => {
    const impl = {
      Minimise: vi.fn().mockResolvedValue(undefined),
      ToggleMaximise: vi.fn().mockResolvedValue(undefined),
      Close: vi.fn().mockResolvedValue(undefined),
      IsMaximised: vi.fn().mockResolvedValue(true),
    };
    const win = createWindowControls(impl);
    await win.minimise();
    await win.toggleMaximise();
    await win.close();
    expect(await win.isMaximised()).toBe(true);
    expect(impl.Minimise).toHaveBeenCalled();
    expect(impl.ToggleMaximise).toHaveBeenCalled();
    expect(impl.Close).toHaveBeenCalled();
  });
});
