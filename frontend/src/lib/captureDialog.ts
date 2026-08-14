import { Dialogs } from '@wailsio/runtime';

const FILTERS = [{ DisplayName: 'UnityPerfAgent Capture (*.upac)', Pattern: '*.upac' }];

function stamp(): string {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}`;
}

export interface CaptureDialogs {
  save(): Promise<string>;
  open(): Promise<string>;
}

export const captureDialogs: CaptureDialogs = {
  async save() {
    const path = await Dialogs.SaveFile({
      Title: '导出 Profiler 数据',
      Filename: `profiler-${stamp()}.upac`,
      Filters: FILTERS,
      CanCreateDirectories: true,
    });
    return Array.isArray(path) ? path[0] ?? '' : path ?? '';
  },
  async open() {
    const path = await Dialogs.OpenFile({
      Title: '加载 Profiler 数据',
      Filters: FILTERS,
      CanChooseFiles: true,
      AllowsOtherFiletypes: true,
    });
    return Array.isArray(path) ? path[0] ?? '' : path ?? '';
  },
};
