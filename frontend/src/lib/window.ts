/** 窗口控制抽象，便于测试时替换 Wails runtime。 */
export interface WindowAPI {
  Minimise(): Promise<void>;
  ToggleMaximise(): Promise<void>;
  Close(): Promise<void>;
  IsMaximised(): Promise<boolean>;
}

export interface WindowControls {
  minimise(): Promise<void>;
  toggleMaximise(): Promise<void>;
  close(): Promise<void>;
  isMaximised(): Promise<boolean>;
}

function fallbackWindow(): WindowControls {
  return {
    minimise: async () => {},
    toggleMaximise: async () => {},
    close: async () => {},
    isMaximised: async () => false,
  };
}

/** 绑定当前 Wails 窗口；未传入时返回空操作（单测 / 浏览器预览）。 */
export function createWindowControls(win?: WindowAPI): WindowControls {
  if (!win) return fallbackWindow();
  return {
    minimise: () => win.Minimise(),
    toggleMaximise: () => win.ToggleMaximise(),
    close: () => win.Close(),
    isMaximised: () => win.IsMaximised(),
  };
}
