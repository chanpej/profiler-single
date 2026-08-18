import { Trash2, Unplug, Usb } from 'lucide-react';
import type { Status } from '../types';

interface Props {
  status: Status;
  busy: boolean;
  packageId: string;
  onPackageIdChange: (value: string) => void;
  onDisconnect: () => void;
  onUSB: () => void;
  onClear: () => void;
}

/** 顶部连接工具栏：包名输入 + USB adb forward。 */
export function ConnectBar({
  status,
  busy,
  packageId,
  onPackageIdChange,
  onDisconnect,
  onUSB,
  onClear,
}: Props) {
  const unityLabel = status.device.unityVersion === '' ? 'Unity 未知' : `Unity ${status.device.unityVersion}`;
  const live = status.connected || !!status.reconnecting;
  const canConnect = packageId.trim() !== '' && !busy && !live;

  return (
    <div className="connect-bar">
      <input
        className="input package-id-input"
        value={packageId}
        onChange={(e) => onPackageIdChange(e.target.value)}
        placeholder="com.company.game"
        aria-label="应用包名"
        title="Android 应用包名，用于 adb forward 到 localabstract:Unity-{包名}"
        spellCheck={false}
        disabled={busy}
      />
      {live ? (
        <>
          <span className={`conn-dot ${status.connected ? 'on' : 'wait'}`} aria-hidden />
          <span className="conn-info">
            {status.reconnecting && !status.connected ? '等待游戏恢复' : status.device.deviceName}
            {status.transport === 'adb' ? ' · USB' : ''}
            {' · '}
            <span
              className={status.unitySupported ? undefined : 'unity-unsupported'}
              title={status.unitySupported ? undefined : '仅官方支持 Unity 2022 Development Build'}
            >
              {unityLabel}
            </span>
          </span>
          <button type="button" className="btn ghost" onClick={onDisconnect} title="断开连接" aria-label="断开连接">
            <Unplug size={16} aria-hidden />
          </button>
        </>
      ) : (
        <>
          <span className="conn-dot" aria-hidden />
          <button
            type="button"
            className="btn primary"
            disabled={!canConnect}
            onClick={onUSB}
            title="USB 连接（adb forward 到 localabstract:Unity-{包名}）"
            aria-label="USB 连接"
          >
            <Usb size={16} aria-hidden />
          </button>
        </>
      )}
      <div className="connect-bar-spacer" />
      <span className="frame-counter mono" title="累计采集帧数">
        {status.totalFrames.toLocaleString()} 帧
      </span>
      <button type="button" className="btn ghost" onClick={onClear} title="清空数据" aria-label="清空数据">
        <Trash2 size={16} aria-hidden />
      </button>
    </div>
  );
}
