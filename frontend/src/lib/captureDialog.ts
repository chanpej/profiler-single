import { Dialogs } from '@wailsio/runtime';
import { isUserCancelled } from './userCancel';

const FILTERS = [{ DisplayName: 'UnityPerfAgent Capture (*.upac)', Pattern: '*.upac' }];

function stamp(): string {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}`;
}

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
  save(): Promise<string>;
  open(): Promise<string>;
}

export const captureDialogs: CaptureDialogs = {
  async save() {
    const path = await ignoreCancel(
      () => Dialogs.SaveFile({
        Title: '导出 Profiler 数据',
        Filename: `profiler-${stamp()}.upac`,
        Filters: FILTERS,
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
        Filters: FILTERS,
        CanChooseFiles: true,
        AllowsOtherFiletypes: true,
      }),
      '',
    );
    return asPath(path);
  },
};
