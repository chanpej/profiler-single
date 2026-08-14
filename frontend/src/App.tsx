import { useCallback, useEffect, useRef, useState } from 'react';
import { Window } from '@wailsio/runtime';
import { api } from './api';
import { captureDialogs } from './lib/captureDialog';
import { loadPackageId, savePackageId } from './lib/packageIdStorage';
import { createWindowControls } from './lib/window';
import type { OverlayId, Stats, Status } from './types';
import { TitleBar } from './components/TitleBar';
import { MenuBar } from './components/MenuBar';
import { AppDialog } from './components/AppDialog';
import { ConnectBar } from './components/ConnectBar';
import { FrameTable } from './components/FrameTable';
import { AnalysisView } from './components/AnalysisView';
import { SettingsView } from './components/SettingsView';
import { ProfilerView } from './components/ProfilerView';

const LIVE_REFRESH_MS = 200;
const IDLE_REFRESH_MS = 500;

const emptyStatus: Status = {
  connected: false,
  mode: 'profiler',
  device: { deviceName: '', deviceModel: '', os: '', unityVersion: '', appVersion: '' },
  totalFrames: 0,
  retained: 0,
  unitySupported: false,
  recording: true,
  sourceFile: '',
  deepProfiling: false,
  callStacks: false,
};

const overlayTitle: Record<OverlayId, string> = {
  frames: '帧数据',
  analysis: '诊断',
  settings: '设置',
};

const windowControls = createWindowControls(Window as unknown as import('./lib/window').WindowAPI);

function App() {
  const [overlay, setOverlay] = useState<OverlayId | null>(null);
  const [status, setStatus] = useState<Status>(emptyStatus);
  const [, setStats] = useState<Stats | null>(null);
  const [busy, setBusy] = useState(false);
  const [packageId, setPackageId] = useState(loadPackageId);
  const [error, setError] = useState('');
  const [llmReady, setLlmReady] = useState(false);
  const lastRefresh = useRef(0);
  const statusRef = useRef(status);
  statusRef.current = status;

  const refresh = useCallback(async () => {
    const [st, sf] = await Promise.all([api.status(), api.stats()]);
    setStatus(st);
    setStats(sf);
  }, []);

  useEffect(() => {
    refresh().catch(() => {});
    api.llmConfig().then((cfg) => setLlmReady((cfg.agentCommand ?? '').trim() !== '')).catch(() => {});
    const off = api.onTick(() => {
      const now = Date.now();
      const live = statusRef.current.connected && statusRef.current.recording !== false;
      const interval = live ? LIVE_REFRESH_MS : IDLE_REFRESH_MS;
      if (now - lastRefresh.current < interval) return;
      lastRefresh.current = now;
      refresh().catch(() => {});
    });
    return off;
  }, [refresh]);

  const guard = useCallback(async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError('');
    try {
      await fn();
      await refresh();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }, [refresh]);

  const onPackageIdChange = useCallback((value: string) => {
    setPackageId(value);
    savePackageId(value);
  }, []);

  const fetchRange = useCallback((offset: number, limit: number) => api.frames(offset, limit), []);
  const getFrame = useCallback((index: number) => api.frameAt(index), []);
  const getHierarchy = useCallback((index: number) => api.frameHierarchy(index), []);
  const saveLLM = useCallback(async (cfg: Parameters<typeof api.setLLMConfig>[0]) => {
    await api.setLLMConfig(cfg);
    setLlmReady(cfg.agentCommand.trim() !== '');
  }, []);

  const recording = status.recording !== false;
  const onToggleRecord = () => {
    if (!status.connected) return;
    guard(() => api.setRecording(!recording));
  };
  const onPause = () => {
    if (!status.connected) return;
    guard(() => api.setRecording(!recording));
  };
  const onExport = () => guard(async () => {
    const path = await captureDialogs.save();
    if (!path) return;
    await api.exportCapture(path);
  });
  const onImport = () => guard(async () => {
    const path = await captureDialogs.open();
    if (!path) return;
    await api.importCapture(path);
  });
  const onClear = () => guard(() => api.clear());
  const onDeepProfiling = (on: boolean) => {
    guard(() => api.setDeepProfiling(on));
  };
  const onCallStacks = (on: boolean) => {
    guard(() => api.setCallStacks(on));
  };

  const fileName = (status.sourceFile ?? '').replace(/^.*[\\/]/, '');
  // 操作报错优先，其次显示采集异常中断的原因（如数据流解析失步）。
  const banner = error || (status.lastError ?? '');

  return (
    <div className="app">
      <TitleBar
        subtitle={fileName}
        controls={windowControls}
      />
      <MenuBar
        canSave={status.retained > 0}
        onImport={onImport}
        onExport={onExport}
        onClear={onClear}
        onFrames={() => setOverlay('frames')}
        onAnalysis={() => setOverlay('analysis')}
        onSettings={() => setOverlay('settings')}
      />
      <div className="app-body">
        <div className="app-main">
          <ConnectBar
            status={status}
            busy={busy}
            packageId={packageId}
            onPackageIdChange={onPackageIdChange}
            onDisconnect={() => guard(() => api.disconnect())}
            onUSB={() => guard(() => api.connectUSB('', packageId.trim()))}
            onClear={onClear}
          />
          {banner && (
            <div className="error-banner" role="alert">{banner}</div>
          )}

          <ProfilerView
            totalCount={status.retained}
            fetchRange={fetchRange}
            getFrame={getFrame}
            getHierarchy={getHierarchy}
            recording={recording}
            connected={status.connected}
            sourceFile={status.sourceFile}
            deepProfiling={status.deepProfiling === true}
            callStacks={status.callStacks === true}
            onClear={onClear}
            onToggleRecord={onToggleRecord}
            onPause={onPause}
            onExport={onExport}
            onImport={onImport}
            onDeepProfiling={onDeepProfiling}
            onCallStacks={onCallStacks}
          />
        </div>
      </div>

      <AppDialog
        title={overlay ? overlayTitle[overlay] : ''}
        open={overlay !== null}
        onClose={() => setOverlay(null)}
        wide={overlay === 'frames'}
      >
        {overlay === 'frames' && (
          <div className="frames-view">
            <FrameTable totalCount={status.retained} fetchRange={fetchRange} />
          </div>
        )}
        {overlay === 'analysis' && (
          <AnalysisView analyze={() => api.analyze()} diagnose={() => api.diagnose()} llmReady={llmReady} />
        )}
        {overlay === 'settings' && <SettingsView load={() => api.llmConfig()} save={saveLLM} />}
      </AppDialog>
    </div>
  );
}

export default App;
