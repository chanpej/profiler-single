import { Dialogs } from '@wailsio/runtime';
import { isUserCancelled } from './userCancel';

const IMPORT_FILTERS = [
  { DisplayName: 'unityperf Capture (*.upac)', Pattern: '*.upac' },
  { DisplayName: 'Unity Profiler Proxy Capture (*.uppc)', Pattern: '*.uppc' },
  { DisplayName: 'Unity Profiler Data (*.data)', Pattern: '*.data' },
];
const EXPORT_FILTERS = [{ DisplayName: 'unityperf Capture (*.upac)', Pattern: '*.upac' }];


function asPath(path: string | string[] | null | undefined): string {
  return Array.isArray(path) ? path[0] ?? '' : path ?? '';
}

async function ignoreCancel<T>(fn: () => Promise<T>, fallback: T): Promise<T> {
  try {
    return await fn();
  } catch (e) {
    if (isUserCancelled(e)) return fallback;
    throw e;
  }
}

export interface CaptureDialogs {
  open(): Promise<string>;
  save(extension?: 'upac' | 'data'): Promise<string>;
  pickEditor(): Promise<string>;
}

export const captureDialogs: CaptureDialogs = {
  async save(extension = 'upac') {
    const path = await ignoreCancel(
      () => Dialogs.SaveFile({
        Title: '保存 Profiler 数据',
        Filename: `profiler-${new Date().toISOString().replace(/[:.]/g, '-')}.${extension}`,
        Filters: extension === 'data' ? [IMPORT_FILTERS[2]] : EXPORT_FILTERS,
        CanCreateDirectories: true,
      }),
      '',
    );
    return asPath(path);
  },
  async pickEditor() {
    return asPath(await ignoreCancel(() => Dialogs.OpenFile({
      Title: '选择与录制兼容的 Unity 或团结 2022 Editor',
      Filters: [{ DisplayName: 'Editor (*.exe)', Pattern: '*.exe' }],
      CanChooseFiles: true,
    }), ''));
  },
  async open() {
    const path = await ignoreCancel(
      () => Dialogs.OpenFile({
        Title: '加载 Profiler 数据',
        Filters: IMPORT_FILTERS,
        CanChooseFiles: true,
      }),
      '',
    );
    return asPath(path);
  },
};
