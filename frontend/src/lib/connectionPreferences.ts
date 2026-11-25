export type ConnectionMode = 'mini' | 'app';
type Setting = 'mode' | 'appIP' | 'wirelessAddress';

export function readSetting(key: Setting): string | undefined {
  try { return localStorage.getItem(`upa.connection.${key}`) ?? undefined; }
  catch { return undefined; }
}

export function writeSetting(key: Setting, value: string) {
  try {
    if (value.trim()) localStorage.setItem(`upa.connection.${key}`, value);
    else localStorage.removeItem(`upa.connection.${key}`);
  } catch { /* Keep the current window usable when storage is unavailable. */ }
}

export function loadMode(): ConnectionMode {
  return readSetting('mode') === 'app' ? 'app' : 'mini';
}
