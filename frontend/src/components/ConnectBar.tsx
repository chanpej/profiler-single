import { useState } from 'react';
import { Trash2, Unplug, Usb } from 'lucide-react';
import type { Status } from '../types';

interface Props {
  status: Status;
  busy: boolean;
  packageId: string;
  onPackageIdChange: (value: string) => void;
  onDisconnect: () => void;
  onUSB: () => void;
  onDirect?: (host: string) => void;
  onAutoconnect?: () => void;
  autoconnectAddresses?: string[];
  onClear: () => void;
  onUnityLogs?: () => void;
}

/** 顶部连接工具栏：USB、手机 IP 直连和小游戏监听。 */
export function ConnectBar({
  status,
  busy,
  packageId,
  onPackageIdChange,
  onDisconnect,
  onUSB,
  onDirect,
  onAutoconnect,
  autoconnectAddresses,
  onClear,
  onUnityLogs,
}: Props) {
  const [deviceIP, setDeviceIP] = useState('');
  const unityLabel = status.device.unityVersion === '' ? 'Unity 未知' : `Unity ${status.device.unityVersion}`;
  const auto = status.transport === 'autoconnect';
  const live = status.connected || !!status.reconnecting;
  const canUSB = !live || (auto && !status.connected);
  const canConnect = packageId.trim() !== '' && !busy && canUSB;
  const addressLabel = autoconnectAddresses?.join(' / ') || '暂无 IPv4';
  const address = autoconnectAddresses && (
    <span className="autoconnect-address mono" title={`本机 IP ${addressLabel}`}>
      本机 IP <span>{addressLabel}</span>
    </span>
  );

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
            {status.reconnecting && !status.connected
              ? (auto ? '等待小游戏连接 · WebSocket 37100' : '等待游戏恢复')
              : status.device.deviceName}
            {status.transport === 'adb' ? ' · USB' : status.transport === 'tcp' ? ' · 局域网' : ''}
            {(!auto || status.connected) && (
              <>
                {' · '}
                <span
                  className={status.unitySupported ? undefined : 'unity-unsupported'}
                  title={status.unitySupported ? undefined : '仅官方支持 Unity 2022 Development Build'}
                >
                  {unityLabel}
                </span>
              </>
            )}
          </span>
          {auto && address}
          {auto ? (
            <button type="button" className="btn minigame-listener-button" disabled={busy} onClick={onDisconnect}
              title="关闭 WebSocket 37100 并断开小游戏连接">
              关闭小游戏监听
            </button>
          ) : (
            <button type="button" className="btn ghost" disabled={busy} onClick={onDisconnect} title="断开连接" aria-label="断开连接">
              <Unplug size={16} aria-hidden />
            </button>
          )}
        </>
      ) : (
        <>
          <span className="conn-dot" aria-hidden />
          {onAutoconnect && (
            <>
              <button type="button" className="btn minigame-listener-button" disabled={busy} onClick={onAutoconnect}
                title="监听 WebSocket 37100，接收 Autoconnected Player 实时采样">
                开启小游戏监听
              </button>
              {address}
            </>
          )}
        </>
      )}
      {canUSB && (
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
      )}
      {onDirect && canUSB && (
        <form className="direct-connect" onSubmit={(event) => {
          event.preventDefault();
          const host = deviceIP.trim();
          if (host && !busy) onDirect(host);
        }}>
          <input className="input host" value={deviceIP} onChange={(event) => setDeviceIP(event.target.value)}
            placeholder="手机 IP" aria-label="手机 IP" spellCheck={false} disabled={busy} />
          <button type="submit" className="btn" disabled={busy || !deviceIP.trim()}>IP 直连</button>
        </form>
      )}
      <div className="connect-bar-spacer" />
      {onUnityLogs && <button type="button" className="btn ghost" onClick={onUnityLogs}>Unity 日志</button>}
      <span className="frame-counter mono" title="累计采集帧数">
        {status.totalFrames.toLocaleString()} 帧
      </span>
      <button type="button" className="btn ghost" onClick={onClear} title="清空数据" aria-label="清空数据">
        <Trash2 size={16} aria-hidden />
      </button>
    </div>
  );
}
