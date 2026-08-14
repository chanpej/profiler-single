const STORAGE_KEY = 'upa.packageId';

/** 读取上次保存的应用包名。 */
export function loadPackageId(): string {
  try {
    return localStorage.getItem(STORAGE_KEY) ?? '';
  } catch {
    return '';
  }
}

/** 持久化应用包名，供下次启动恢复。 */
export function savePackageId(value: string): void {
  try {
    localStorage.setItem(STORAGE_KEY, value);
  } catch {
    // WebView 存储不可用时忽略。
  }
}
