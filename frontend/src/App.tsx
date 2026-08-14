import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from './api';
import { loadPackageId, savePackageId } from './lib/packageIdStorage';
import type { FrameData, Stats, Status, ViewId } from './types';
import { Sidebar } from './components/Sidebar';
import { ConnectBar } from './components/ConnectBar';
import { FrameTable } from './components/FrameTable';
import { AnalysisView } from './components/AnalysisView';
import { SettingsView } from './components/SettingsView';
import { ProfilerView } from './components/ProfilerView';

const CHART_WINDOW = 600;
const REFRESH_MS = 500;

const emptyStatus: Status = {
  connected: false,
  mode: 'profiler',
  device: { deviceName: '', deviceModel: '', os: '', unityVersion: '', appVersion: '' },
  totalFrames: 0,
  retained: 0,
  unitySupported: false,
};

function App() {
  const [view, setView] = useState<ViewId>('dashboard');
  const [status, setStatus] = useState<Status>(emptyStatus);
  const [, setStats] = useState<Stats | null>(null);
  const [chartFrames, setChartFrames] = useState<FrameData[]>([]);
  const [busy, setBusy] = useState(false);
  const [packageId, setPackageId] = useState(loadPackageId);
  const [error, setError] = useState('');
  const [llmReady, setLlmReady] = useState(false);
  const lastRefresh = useRef(0);

  const refresh = useCallback(async () => {
    const [st, sf, latest] = await Promise.all([api.status(), api.stats(), api.latest(CHART_WINDOW)]);
    setStatus(st);
    setStats(sf);
    setChartFrames(latest ?? []);
  }, []);

  useEffect(() => {
    refresh().catch(() => {});
    api.llmConfig().then((cfg) => setLlmReady((cfg.agentCommand ?? '').trim() !== '')).catch(() => {});
    const off = api.onTick(() => {
      const now = Date.now();
      if (now - lastRefresh.current < REFRESH_MS) return;
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
  const saveLLM = useCallback(async (cfg: Parameters<typeof api.setLLMConfig>[0]) => {
    await api.setLLMConfig(cfg);
    setLlmReady(cfg.agentCommand.trim() !== '');
  }, []);

  return (
    <div className="app">
      <Sidebar active={view} onSelect={setView} />
      <div className="app-main">
        <ConnectBar
          status={status}
          busy={busy}
          packageId={packageId}
          onPackageIdChange={onPackageIdChange}
          onDisconnect={() => guard(() => api.disconnect())}
          onUSB={() => guard(() => api.connectUSB('', packageId.trim()))}
          onClear={() => guard(() => api.clear())}
        />
        {error && (
          <div className="error-banner" role="alert">{error}</div>
        )}

        {view === 'dashboard' && (
          <ProfilerView
            frames={chartFrames}
            recording={status.connected}
            onClear={() => guard(() => api.clear())}
          />
        )}

        {view === 'frames' && (
          <div className="frames-view">
            <FrameTable totalCount={status.retained} fetchRange={fetchRange} />
          </div>
        )}

        {view === 'analysis' && (
          <AnalysisView analyze={() => api.analyze()} diagnose={() => api.diagnose()} llmReady={llmReady} />
        )}

        {view === 'settings' && <SettingsView load={() => api.llmConfig()} save={saveLLM} />}
      </div>
    </div>
  );
}

export default App;
