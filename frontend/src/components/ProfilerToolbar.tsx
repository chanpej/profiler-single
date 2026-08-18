import { ChevronLeft, ChevronRight, Circle, Eraser, FolderOpen, Layers, ListTree, MousePointerClick, Pause, Play, SkipForward } from 'lucide-react';
import { formatFps, formatGCAlloc, formatMs } from '../lib/format';
import type { FrameData } from '../types';

interface Props {
  recording: boolean;
  connected: boolean;
  frame: FrameData | null;
  selectedIndex: number;
  total: number;
  deepProfiling?: boolean;
  callStacks?: boolean;
  onClear: () => void;
  onPrev: () => void;
  onNext: () => void;
  onSeek: (index: number) => void;
  onCurrent: () => void;
  onToggleRecord: () => void;
  onPause: () => void;

  onImport: () => void;
  onDeepProfiling?: (on: boolean) => void;
  onCallStacks?: (on: boolean) => void;
}

/** Unity Profiler 2022 风格顶栏：Record / Pause / Deep Profile / Call Stacks / Clear / Load / Save / 帧导航。 */
export function ProfilerToolbar({
  recording, connected, frame, selectedIndex, total,
  deepProfiling = false, callStacks = false,
  onClear, onPrev, onNext, onSeek, onCurrent, onToggleRecord, onPause, onImport,
  onDeepProfiling, onCallStacks,
}: Props) {
  const hasSelection = total > 0 && selectedIndex >= 0;
  const atStart = !hasSelection || selectedIndex <= 0;
  const atEnd = !hasSelection || selectedIndex >= total - 1;
  const paused = connected && !recording;
  const canCapture = connected;

  return (
    <div className="profiler-toolbar" role="toolbar" aria-label="Profiler 工具栏">
      <button
        type="button"
        className={`btn ghost profiler-record ${recording && connected ? 'is-on' : ''}`}
        aria-pressed={recording && connected}
        title={connected ? (recording ? '停止录制' : '开始录制') : '连接 Player 后才能录制'}
        aria-label={recording && connected ? '正在采集' : '未采集'}
        disabled={!canCapture}
        onClick={onToggleRecord}
      >
        <Circle size={12} fill={recording && connected ? 'currentColor' : 'none'} aria-hidden />
      </button>
      <button
        type="button"
        className={`btn ghost profiler-pause ${paused ? 'is-on' : ''}`}
        aria-pressed={paused}
        title={paused ? '继续抓取数据' : '暂停抓取数据'}
        aria-label={paused ? '继续抓取' : '暂停抓取'}
        disabled={!canCapture}
        onClick={onPause}
      >
        {paused ? <Play size={14} aria-hidden /> : <Pause size={14} aria-hidden />}
      </button>
      <label
        className={`profiler-check ${connected ? '' : 'is-disabled'}`}
        title={connected
          ? '采集全部脚本方法调用。注意：该开关对应的报文尚未逆向出来，目前只记录状态、不会下发到设备'
          : '连接 Player 后可开启 Deep Profile'}
      >
        <input
          type="checkbox"
          aria-label="Deep Profile"
          checked={deepProfiling}
          disabled={!connected}
          onChange={(e) => onDeepProfiling?.(e.target.checked)}
        />
        Deep Profile
      </label>
      <label
        className={`profiler-check ${connected ? '' : 'is-disabled'}`}
        title={connected
          ? '记录分配采样的调用栈（托管 GC.Alloc 与引擎原生分配），只对开启之后采集的帧生效'
          : '连接 Player 后可开启 Call Stacks'}
      >
        <input
          type="checkbox"
          aria-label="Call Stacks"
          checked={callStacks}
          disabled={!connected}
          onChange={(e) => onCallStacks?.(e.target.checked)}
        />
        <ListTree size={12} aria-hidden />
        Call Stacks
      </label>
      <button type="button" className="btn ghost" onClick={onClear} title="清空数据" aria-label="清空数据">
        <Eraser size={14} aria-hidden />
      </button>
      <button type="button" className="btn ghost" onClick={onImport} title="加载二进制数据" aria-label="加载数据">
        <FolderOpen size={14} aria-hidden />
      </button>

      <span className="profiler-sep" aria-hidden />
      <button type="button" className="btn ghost" disabled={atStart} onClick={onPrev} title="上一帧" aria-label="上一帧">
        <ChevronLeft size={16} aria-hidden />
      </button>
      <input
        className="input profiler-frame-input"
        aria-label="当前帧"
        value={hasSelection ? String(selectedIndex + 1) : ''}
        onChange={(e) => {
          const n = Number.parseInt(e.target.value, 10);
          if (Number.isInteger(n) && n >= 1) onSeek(n - 1);
        }}
        disabled={total === 0}
      />
      <span className="dim">/ {total}</span>
      <button type="button" className="btn ghost" disabled={atEnd} onClick={onNext} title="下一帧" aria-label="下一帧">
        <ChevronRight size={16} aria-hidden />
      </button>
      <button type="button" className="btn ghost" disabled={total === 0} onClick={onCurrent} title="跳到最新帧" aria-label="最新帧">
        <SkipForward size={14} aria-hidden />
      </button>
      <span className="profiler-sep" aria-hidden />
      {frame ? (
        <>
          <Layers size={14} className="dim" aria-hidden />
          <span className="mono profiler-frame-meta">
            CPU {formatMs(frame.cpuTimeMs)}
            {'  '}
            {formatFps(frame.fps)} FPS
            {frame.gcAllocKB > 0 ? `  GC ${formatGCAlloc(frame.gcAllocKB)}` : ''}
          </span>
        </>
      ) : (
        <span className="profiler-frame-empty" title="无选中帧" aria-label="无选中帧">
          <MousePointerClick size={14} aria-hidden />
        </span>
      )}
    </div>
  );
}
