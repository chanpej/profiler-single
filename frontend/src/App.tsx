import { useCallback, useEffect, useRef, useState } from 'react';
import { Window } from '@wailsio/runtime';
import { api } from './api';
import { captureDialogs } from './lib/captureDialog';
import { loadPackageId, savePackageId } from './lib/packageIdStorage';
import { createWindowControls } from './lib/window';
import { formatErrorMessage, isUserCancelled } from './lib/userCancel';
import { loadAgentCollapsed, loadAgentWidth, saveAgentCollapsed } from './lib/agentPanel';
import { applyChatFontSize, loadChatFontSize } from './lib/chatAppearance';
import { emptyLLMConfig, isAgentConfigured, normalizeLLMConfig, updateAgent, withActiveAgent } from './lib/llmConfig';
import { applyLLMRuntime, loadLLMRuntime, saveLLMRuntime } from './lib/llmRuntimeStorage';
import { initialImportProgress } from './lib/importProgress';
import type { AgentProfile, ImportProgressEvent, LLMConfig, OverlayId, Stats, Status } from './types';
import { TitleBar } from './components/TitleBar';
import { MenuBar } from './components/MenuBar';
import { AppDialog } from './components/AppDialog';
import { ErrorPopup } from './components/ErrorPopup';
import { ProgressPopup } from './components/ProgressPopup';
import { ConnectBar } from './components/ConnectBar';
import { FrameTable } from './components/FrameTable';
import { AnalysisView } from './components/AnalysisView';
import { SettingsView } from './components/SettingsView';
import { ProfilerView } from './components/ProfilerView';
import { AgentChatPanel } from './components/AgentChatPanel';
import { StatusBar } from './components/StatusBar';

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
  const [importProgress, setImportProgress] = useState<ImportProgressEvent | null>(null);
  const [llm, setLlm] = useState<LLMConfig>(() => applyLLMRuntime(emptyLLMConfig, loadLLMRuntime()));
  const [agentCollapsed, setAgentCollapsed] = useState(loadAgentCollapsed);
  const [agentWidth, setAgentWidth] = useState(loadAgentWidth);
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
    applyChatFontSize(loadChatFontSize());
    refresh().catch(() => {});
    api.llmConfig().then(async (cfg) => {
      const next = applyLLMRuntime(cfg, loadLLMRuntime());
      setLlm(next);
      saveLLMRuntime(next);
      await api.setLLMConfig(next);
    }).catch(() => {});
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
  const saveLLM = useCallback(async (cfg: Parameters<typeof api.setLLMConfig>[0]) => {
    const next = normalizeLLMConfig(cfg);
    saveLLMRuntime(next);
    await api.setLLMConfig(next);
    setLlm(next);
  }, []);

  const onSelectAgent = useCallback((id: string) => {
    setLlm((cur) => {
      const next = withActiveAgent(cur, id);
      saveLLMRuntime(next);
      api.setLLMConfig(next).catch(() => {});
      return next;
    });
  }, []);

  const onUpdateAgent = useCallback((patch: Partial<AgentProfile> & { id: string }) => {
    setLlm((cur) => {
      const next = updateAgent(cur, patch);
      saveLLMRuntime(next);
      api.setLLMConfig(next).catch(() => {});
      return next;
    });
  }, []);

  const onToggleAgent = useCallback(() => {
    setAgentCollapsed((cur) => {
      const next = !cur;
      saveAgentCollapsed(next);
      return next;
    });
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
  const onClear = () => guard(() => api.clear());
  const onDeepProfiling = (on: boolean) => {
    guard(() => api.setDeepProfiling(on));
  };
  const onCallStacks = (on: boolean) => {
    guard(() => api.setCallStacks(on));
  };

  const llmReady = isAgentConfigured(llm);

  return (
    <div className="app">
      <TitleBar controls={windowControls} />
      <MenuBar
        onImport={onImport}
        onClear={onClear}
        onFrames={() => setOverlay('frames')}
        onAnalysis={() => setOverlay('analysis')}
        onSettings={() => setOverlay('settings')}
        onOpenAgentLog={() => { api.openAgentLog().catch((e) => setError(formatErrorMessage(e))); }}
        onToggleAgent={onToggleAgent}
        agentCollapsed={agentCollapsed}
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

          <ProfilerView
            totalCount={status.retained}
            fetchRange={fetchRange}
            getFrame={getFrame}
            getHierarchy={getHierarchy}
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
        <AgentChatPanel
          llm={llm}
          collapsed={agentCollapsed}
          width={agentWidth}
          onToggle={onToggleAgent}
          onWidthChange={setAgentWidth}
          onSelectAgent={onSelectAgent}
          onUpdateAgent={onUpdateAgent}
          listModels={api.listAgentModels}
          agentChatSend={api.agentChatSend}
          onChunk={api.onAgentChunk}
        />
      </div>
      <StatusBar status={status} busy={busy} agentName={llmReady ? (llm.agents.find((a) => a.id === llm.activeId)?.name ?? '') : ''} />

      <ErrorPopup message={error} open={error !== ''} onClose={() => setError('')} />
      <ProgressPopup progress={importProgress} />

      <AppDialog
        title={overlay ? overlayTitle[overlay] : ''}
        open={overlay !== null}
        onClose={() => setOverlay(null)}
        wide={overlay === 'frames' || overlay === 'settings'}
      >
        {overlay === 'frames' && (
          <div className="frames-view">
            <FrameTable totalCount={status.retained} fetchRange={fetchRange} />
          </div>
        )}
        {overlay === 'analysis' && (
          <AnalysisView analyze={() => api.analyze()} diagnose={() => api.diagnose()} llmReady={llmReady} />
        )}
        {overlay === 'settings' && (
          <SettingsView load={() => api.llmConfig()} save={saveLLM} probe={api.probeAgent} />
        )}
      </AppDialog>
    </div>
  );
}

export default App;
