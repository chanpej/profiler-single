import { useState } from 'react';
import type { MemorySample } from '../../bindings/unityperf/services/models';
import { isUserCancelled } from '../lib/userCancel';

export const snapshotCaptured = 'unityperf-memory-captured';

export function MemoryCaptureButton({ miniGame, connected }: { miniGame?: boolean; connected?: boolean }) {
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  async function capture() {
    setBusy(true); setMessage('');
    try {
      const service = await import('../../bindings/unityperf/services/perfservice');
      const sample = await (miniGame ? service.CaptureMiniGameMemory() : service.CaptureUnityMemory());
      window.dispatchEvent(new CustomEvent<MemorySample>(snapshotCaptured, { detail: sample }));
      setMessage('已加入当前会话，保存 UPAC 时一并写入');
    } catch (reason) { if (!isUserCancelled(reason)) setMessage(String(reason)); }
    finally { setBusy(false); }
  }
  return <><button type="button" className="btn ghost" disabled={busy || (miniGame && !connected)} onClick={capture}>Memory Profiler Capture</button>
    {(busy || message) && <small role="status">{busy ? '正在采集内存快照…' : message}</small>}</>;
}
