import { useCallback, useEffect, useRef, useState } from 'react';
import { Window } from '@wailsio/runtime';
import { api } from './api';
import { captureDialogs } from './lib/captureDialog';
import { loadPackageId, savePackageId } from './lib/packageIdStorage';
import { createWindowControls } from './lib/window';
import { formatErrorMessage, isUserCancelled } from './lib/userCancel';
import { initialImportProgress } from './lib/importProgress';
import type { ImportProgressEvent, OverlayId, Stats, Status } from './types';
import { TitleBar } from './components/TitleBar';
import { MenuBar } from './components/MenuBar';
import { AppDialog } from './components/AppDialog';
import { ErrorPopup } from './components/ErrorPopup';
import { ProgressPopup } from './components/ProgressPopup';
import { ConnectBar } from './components/ConnectBar';
import { FrameTable } from './components/FrameTable';
import { AnalysisView } from './components/AnalysisView';
import { ProfilerView } from './components/ProfilerView';
import { StatusBar } from './components/StatusBar';
import { UnityLogView } from './components/UnityLogView';

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
  logs: 'Unity 日志',
};

const windowControls = createWindowControls(Window as unknown as import('./lib/window').WindowAPI);

function App() {
  const [overlay, setOverlay] = useState<OverlayId | null>(null);
  const [status, setStatus] = useState<Status>(emptyStatus);
  const [, setStats] = useState<Stats | null>(null);
  const [busy, setBusy] = useState(false);
  const [packageId, setPackageId] = useState(loadPackageId);
  const [autoconnectAddresses, setAutoconnectAddresses] = useState<string[]>([]);
  const [error, setError] = useState('');
  const [importProgress, setImportProgress] = useState<ImportProgressEvent | null>(null);
  const lastRefresh = useRef(0);
  const statusRef = useRef(status);
  const shownLastError = useRef('');
  const importing = useRef(false);
  statusRef.current = status;

  const refresh = useCallback(async () => {
    const [st, sf] = await Promise.all([api.status(), api.stats()]);
    setStatus(st);
    setStats(sf);
  }, []);

  useEffect(() => {
    refresh().catch(() => {});
    const offTick = api.onTick(() => {
      const now = Date.now();
      const live = statusRef.current.connected && statusRef.current.recording !== false;
      const interval = live ? LIVE_REFRESH_MS : IDLE_REFRESH_MS;
      if (now - lastRefresh.current < interval) return;
      lastRefresh.current = now;
      refresh().catch(() => {});
    });
    const offImport = api.onImportProgress((ev) => {
      if (importing.current) setImportProgress(ev);
    });
    return () => {
      offTick();
      offImport();
    };
  }, [refresh]);

  useEffect(() => {
    const le = (status.lastError ?? '').trim();
    if (!le) {
      shownLastError.current = '';
      return;
    }
    if (le !== shownLastError.current) {
      shownLastError.current = le;
      setError(le);
    }
  }, [status.lastError]);

  useEffect(() => {
    let request = 0;
    const refreshAddresses = () => {
      const current = ++request;
      api.autoconnectAddresses().then((addresses) => {
        if (current === request) setAutoconnectAddresses(addresses);
      }).catch(() => {});
    };
    refreshAddresses();
    window.addEventListener('focus', refreshAddresses);
    return () => {
      request++;
      window.removeEventListener('focus', refreshAddresses);
    };
  }, [status.transport]);

  const guard = useCallback(async (fn: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await fn();
      await refresh();
    } catch (e) {
      if (!isUserCancelled(e)) setError(formatErrorMessage(e));
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
  const getTimeline = useCallback((index: number, frameIndex: number) => api.frameTimeline(index, frameIndex), []);
  const recording = status.recording !== false;
  const onToggleRecord = () => {
    if (!status.connected) return;
    guard(() => api.setRecording(!recording));
  };
  const onPause = () => {
    if (!status.connected) return;
    guard(() => api.setRecording(!recording));
  };

  const onImport = () => guard(async () => {
    const path = await captureDialogs.open();
    if (!path) return;
    importing.current = true;
    setImportProgress(initialImportProgress(path));
    try {
      await api.importCapture(path);
    } finally {
      importing.current = false;
      setImportProgress(null);
    }
  });
  const onExport = () => guard(async () => {
    const path = await captureDialogs.save();
    if (path) await api.exportCapture(path);
  });
  const [exportingUnityData, setExportingUnityData] = useState(false);
  const onExportUnityData = () => guard(async () => {
    if (exportingUnityData) return;
    setExportingUnityData(true);
    try {
      const path = await captureDialogs.save('data');
      if (!path) return;
      const editor = await captureDialogs.pickEditor();
      if (editor) await api.exportUnityData(path, editor);
    } finally {
      setExportingUnityData(false);
    }
  });
  const onClear = () => guard(() => api.clear());
  const onDeepProfiling = (on: boolean) => {
    guard(() => api.setDeepProfiling(on));
  };
  const onCallStacks = (on: boolean) => {
    guard(() => api.setCallStacks(on));
  };
  return (
    <div className="app">
      <TitleBar controls={windowControls} />
      <MenuBar
        onImport={onImport}
        onExport={onExport}
        onExportUnityData={onExportUnityData}
        exportingUnityData={exportingUnityData}
        onClear={onClear}
        onFrames={() => setOverlay('frames')}
        onAnalysis={() => setOverlay('analysis')}
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
            onDirect={(host) => guard(() => api.connectDirect(host))}
            onAutoconnect={() => guard(() => api.startAutoconnect(37100))}
            autoconnectAddresses={autoconnectAddresses}
            onClear={onClear}
            onUnityLogs={() => setOverlay('logs')}
          />

          <ProfilerView
            totalCount={status.retained}
            fetchRange={fetchRange}
            getFrame={getFrame}
            getHierarchy={getHierarchy}
            getTimeline={getTimeline}
            getHeavyProfile={api.frameHeavyProfile}
            getScreenshots={api.screenFilmstrip}
            setScreenCapture={api.setScreenCapture}
            recording={recording}
            connected={status.connected}
            deepProfiling={status.deepProfiling === true}
            callStacks={status.callStacks === true}
            onClear={onClear}
            onToggleRecord={onToggleRecord}
            onPause={onPause}

            onImport={onImport}
            onDeepProfiling={onDeepProfiling}
            onCallStacks={onCallStacks}
          />
        </div>
      </div>
      <StatusBar status={status} busy={busy} />

      <ErrorPopup message={error} open={error !== ''} onClose={() => setError('')} />
      <ProgressPopup progress={importProgress} />

      <AppDialog
        title={overlay ? overlayTitle[overlay] : ''}
        open={overlay !== null}
        onClose={() => setOverlay(null)}
        wide={overlay === 'frames' || overlay === 'logs'}
      >
        {overlay === 'frames' && (
          <div className="frames-view">
            <FrameTable totalCount={status.retained} fetchRange={fetchRange} />
          </div>
        )}
        {overlay === 'analysis' && (
          <AnalysisView analyze={api.analyze} />
        )}
        {overlay === 'logs' && <UnityLogView fetchLogs={api.playerLogs} miniGame={status.transport === 'autoconnect'} />}
      </AppDialog>
    </div>
  );
}

export default App;
