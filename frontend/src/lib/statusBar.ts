import type { Status } from '../types';

/** 状态栏连接文案。 */
export function connectionLabel(status: Status): string {
  if (!status.connected) return '未连接';
  return status.transport === 'adb' ? 'USB 已连接' : '已连接';
}

/** 状态栏采集文案。 */
export function recordingLabel(status: Status): string {
  if (!status.connected) return '空闲';
  return status.recording === false ? '已暂停' : '采集中';
}

/** 状态栏帧计数文案。 */
export function frameCountLabel(status: Status): string {
  const retained = status.retained.toLocaleString();
  const total = status.totalFrames.toLocaleString();
  if (status.retained === status.totalFrames) return `${retained} 帧`;
  return `${retained} / ${total} 帧`;
}
