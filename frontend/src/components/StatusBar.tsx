import { Circle, Pause } from 'lucide-react';
import { connectionLabel, frameCountLabel, recordingLabel } from '../lib/statusBar';
import type { Status } from '../types';

interface Props {
  status: Status;
  busy?: boolean;
}

/** 底部状态栏：连接、采集、帧数。 */
export function StatusBar({ status, busy = false }: Props) {
  const connected = status.connected;
  const reconnecting = !!status.reconnecting;
  const paused = connected && status.recording === false;
  const rec = recordingLabel(status);
  const fileName = (status.sourceFile ?? '').replace(/^.*[\\/]/, '');

  return (
    <footer className="statusbar" data-testid="statusbar" role="status">
      <span className={`statusbar-item ${connected ? 'is-on' : reconnecting ? 'is-warn' : ''}`}>
        <span className={`conn-dot ${connected ? 'on' : reconnecting ? 'wait' : ''}`} aria-hidden />
        {connectionLabel(status)}
      </span>
      <span className="statusbar-sep" aria-hidden />
      <span className={`statusbar-item ${paused || reconnecting ? 'is-warn' : connected ? 'is-on' : ''}`}>
        {paused || reconnecting ? <Pause size={11} aria-hidden /> : <Circle size={8} aria-hidden />}
        {rec}
      </span>
      <span className="statusbar-sep" aria-hidden />
      <span className="statusbar-item mono">{frameCountLabel(status)}</span>
      {status.device.unityVersion ? (
        <>
          <span className="statusbar-sep" aria-hidden />
          <span className={`statusbar-item ${status.unitySupported ? '' : 'is-warn'}`}>
            Unity {status.device.unityVersion}
          </span>
        </>
      ) : null}
      {fileName ? (
        <>
          <span className="statusbar-sep" aria-hidden />
          <span className="statusbar-item" title={status.sourceFile}>{fileName}</span>
        </>
      ) : null}
      <span className="statusbar-spacer" />
      {busy ? <span className="statusbar-item">处理中…</span> : null}
    </footer>
  );
}
