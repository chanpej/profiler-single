import { ChevronLeft, ChevronRight, Circle, Eraser, Layers, ListTree } from 'lucide-react';
import { formatFps, formatMs } from '../lib/format';
import type { FrameData } from '../types';

interface Props {
  recording: boolean;
  frame: FrameData | null;
  selectedIndex: number;
  total: number;
  onClear: () => void;
  onPrev: () => void;
  onNext: () => void;
  onSeek: (index: number) => void;
  onCurrent: () => void;
}

/** Unity Profiler 风格顶栏：Record / Deep Profile / Call Stacks / Clear / 帧导航。 */
export function ProfilerToolbar({
  recording, frame, selectedIndex, total, onClear, onPrev, onNext, onSeek, onCurrent,
}: Props) {
  const atStart = selectedIndex <= 0 || total === 0;
  const atEnd = selectedIndex >= total - 1 || total === 0;

  return (
    <div className="profiler-toolbar" role="toolbar" aria-label="Profiler 工具栏">
      <button
        type="button"
        className={`btn ghost profiler-record ${recording ? 'is-on' : ''}`}
        aria-pressed={recording}
        title={recording ? '正在采集' : '未采集'}
        aria-label={recording ? '正在采集' : '未采集'}
        disabled
      >
        <Circle size={12} fill={recording ? 'currentColor' : 'none'} aria-hidden />
        Record
      </button>
      <label className="profiler-check is-disabled" title="Deep Profile 需要 Unity Editor 插桩，独立工具暂不可用">
        <input type="checkbox" disabled aria-label="Deep Profile" />
        Deep Profile
      </label>
      <label className="profiler-check is-disabled" title="Call Stacks 需要 Unity Editor 插桩，独立工具暂不可用">
        <input type="checkbox" disabled aria-label="Call Stacks" />
        <ListTree size={12} aria-hidden />
        Call Stacks
      </label>
      <button type="button" className="btn ghost" onClick={onClear} title="清空数据" aria-label="清空数据">
        <Eraser size={14} aria-hidden />
        Clear
      </button>
      <span className="profiler-sep" aria-hidden />
      <button type="button" className="btn ghost" disabled={atStart} onClick={onPrev} title="上一帧" aria-label="上一帧">
        <ChevronLeft size={16} aria-hidden />
      </button>
      <input
        className="input profiler-frame-input"
        aria-label="当前帧"
        value={total === 0 ? '' : String(selectedIndex)}
        onChange={(e) => {
          const n = Number.parseInt(e.target.value, 10);
          if (Number.isInteger(n)) onSeek(n);
        }}
        disabled={total === 0}
      />
      <span className="dim">/ {Math.max(total - 1, 0)}</span>
      <button type="button" className="btn ghost" disabled={atEnd} onClick={onNext} title="下一帧" aria-label="下一帧">
        <ChevronRight size={16} aria-hidden />
      </button>
      <button type="button" className="btn ghost" disabled={total === 0} onClick={onCurrent} title="跳到最新帧" aria-label="最新帧">
        Current
      </button>
      <span className="profiler-sep" aria-hidden />
      <Layers size={14} className="dim" aria-hidden />
      <span className="mono profiler-frame-meta">
        {frame ? (
          <>
            #{frame.frameIndex}
            {' · '}
            {formatMs(frame.cpuTimeMs)}
            {' · '}
            {formatFps(frame.fps)} FPS
          </>
        ) : (
          '无选中帧'
        )}
      </span>
    </div>
  );
}
