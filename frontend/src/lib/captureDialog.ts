import { Dialogs } from '@wailsio/runtime';
import { isUserCancelled } from './userCancel';

const FILTERS = [{ DisplayName: 'Unity Profiler Data (*.data)', Pattern: '*.data' }];


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
}

export const captureDialogs: CaptureDialogs = {
  async open() {
    const path = await ignoreCancel(
      () => Dialogs.OpenFile({
        Title: '加载 Profiler 数据',
        Filters: FILTERS,
        CanChooseFiles: true,
      }),
      '',
    );
    return asPath(path);
  },
};
