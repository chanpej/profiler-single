import { Dialogs } from '@wailsio/runtime';
import { isUserCancelled } from './userCancel';

const IMPORT_FILTERS = [
  { DisplayName: 'UnityPerfAgent Capture (*.upac)', Pattern: '*.upac' },
  { DisplayName: 'Unity Profiler Data (*.data)', Pattern: '*.data' },
];
const EXPORT_FILTERS = [{ DisplayName: 'UnityPerfAgent Capture (*.upac)', Pattern: '*.upac' }];


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
  save(): Promise<string>;
}

export const captureDialogs: CaptureDialogs = {
  async save() {
    const path = await ignoreCancel(
      () => Dialogs.SaveFile({
        Title: '保存 Profiler 数据',
        Filename: `profiler-${new Date().toISOString().replace(/[:.]/g, '-')}.upac`,
        Filters: EXPORT_FILTERS,
        CanCreateDirectories: true,
      }),
      '',
    );
    return asPath(path);
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
