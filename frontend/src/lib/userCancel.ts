/** 用户取消系统文件对话框时，Wails 会抛出这类错误。 */
export function isUserCancelled(err: unknown): boolean {
  const msg = String(err).toLowerCase();
  if (msg.includes('cancelled by user') || msg.includes('canceled by user')) return true;
  return /dialog\.\w+ failed/.test(msg) && /cancell?ed/.test(msg);
}

/** 去掉 Error: 前缀，便于弹窗展示。 */
export function formatErrorMessage(err: unknown): string {
  return String(err).replace(/^error:\s*/i, '').trim();
}
