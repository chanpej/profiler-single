import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { CpuTimelineFrame, FrameData, FrameRange, HeavyProfile, ProfilerModuleId, SampleNode } from '../types';
import { DEFAULT_CPU_CHART_MAX_Y, DEFAULT_ENABLED, PROFILER_MODULES } from '../lib/profiler';
import { clampDetailHeight, loadDetailHeight, saveDetailHeight } from '../lib/detailHeight';
import {
  DEFAULT_VISIBLE,
  clampViewport,
  goCurrent,
  localIndexFromX,
  maxStart,
  panViewport,
  seekTo,
  selectInView,
  selectedInWindow,
  setViewportStart,
  wheelPanDelta,
  wheelIntent,
  wheelZoomFactor,
  zoomViewport,
  type Viewport,
} from '../lib/viewport';
import { ProfilerToolbar } from './ProfilerToolbar';
import { ScreenFilmstrip, type ScreenFilmstripData } from './ScreenFilmstrip';
import { ProfilerChart } from './ProfilerChart';
import { ProfilerDetail, type CpuTab } from './ProfilerDetail';

interface Props {
  /** 全量帧（测试或小数据）；与 fetchRange 二选一。 */
  frames?: FrameData[];
  totalCount?: number;
  fetchRange?: (offset: number, limit: number) => Promise<FrameData[]>;
  getFrame?: (index: number) => Promise<FrameData>;
  getHierarchy?: (index: number) => Promise<SampleNode[]>;
  getTimeline?: (index: number, frameIndex: number) => Promise<CpuTimelineFrame[]>;
  getHeavyProfile?: (start: number, end: number) => Promise<HeavyProfile>;
  getScreenshots?: (offset: number, count: number, selected: number) => Promise<ScreenFilmstripData>;
  setScreenCapture?: (enabled: boolean) => Promise<void>;
  recording: boolean;
  connected: boolean;
  deepProfiling?: boolean;
  callStacks?: boolean;
  onClear: () => void;
  onToggleRecord: () => void;
  onPause: () => void;

  onImport: () => void;
  onDeepProfiling?: (on: boolean) => void;
  onCallStacks?: (on: boolean) => void;
}

const initialVp: Viewport = {
  start: 0,
  visible: DEFAULT_VISIBLE,
  selected: 0,
  follow: true,
};

/** Unity Profiler 2022 主视图：实时滚动 / 暂停视野 / 滚轮缩放。 */
export function ProfilerView({
  frames, totalCount, fetchRange, getFrame, getHierarchy, getTimeline, getHeavyProfile, recording, connected,
  getScreenshots, setScreenCapture,
  deepProfiling = false, callStacks = false,
  onClear, onToggleRecord, onPause, onImport, onDeepProfiling, onCallStacks,
}: Props) {
  const [enabled, setEnabled] = useState<ProfilerModuleId[]>(DEFAULT_ENABLED);
  const [active, setActive] = useState<ProfilerModuleId>('cpu');
  const [vp, setVp] = useState<Viewport>(initialVp);
  const [slice, setSlice] = useState<FrameData[]>([]);
  const sliceStart = useRef(0);
  const [pickedFrame, setPickedFrame] = useState<{ index: number; frame: FrameData } | null>(null);
  const [hierarchy, setHierarchy] = useState<SampleNode[]>([]);
  const [heavyRange, setHeavyRange] = useState<FrameRange | null>(null);
  const [timelineRange, setTimelineRange] = useState<FrameRange | null>(null);
  const [cpuTab, setCpuTab] = useState<CpuTab>('hierarchy');
  const [detailH, setDetailH] = useState(loadDetailHeight);
  const [yAxisMax, setYAxisMax] = useState<Partial<Record<ProfilerModuleId, number>>>({
    gpu: DEFAULT_CPU_CHART_MAX_Y,
    rendering: DEFAULT_CPU_CHART_MAX_Y,
  });
  const drag = useRef<{ startY: number; startH: number } | null>(null);

  const total = totalCount ?? frames?.length ?? 0;
  const validHeavyRange = heavyRange && heavyRange.end < total ? heavyRange : null;
  const cpuRange = cpuTab === 'timeline' ? timelineRange : cpuTab === 'left-heavy' ? validHeavyRange : null;
  const syncTimelineRange = useCallback((range: FrameRange | null) => {
    setTimelineRange(range);
    if (!range) return;
    setVp((previous) => {
      const current = clampViewport(previous, total);
      if (range.end >= current.start && range.start < current.start + current.visible) return previous;
      const start = range.start < current.start ? range.start : range.end - current.visible + 1;
      return { ...previous, start: Math.max(0, start), selected: current.selected, follow: false };
    });
  }, [total]);
  const windowed = useMemo(() => clampViewport(vp, total), [vp, total]);
  const localSelected = selectedInWindow(windowed);
  const picked = frames?.[windowed.selected] ?? (pickedFrame?.index === windowed.selected ? pickedFrame.frame : null);

  useEffect(() => {
    setVp((cur) => ({ ...clampViewport(cur, total), visible: cur.visible }));
  }, [total]);

  useEffect(() => {
    if (frames) {
      sliceStart.current = windowed.start;
      setSlice(frames.slice(windowed.start, windowed.start + windowed.visible));
      return;
    }
    if (!fetchRange || total === 0) {
      setSlice([]);
      return;
    }
    let live = true;
    fetchRange(windowed.start, windowed.visible).then((rows) => {
      if (live) {
        sliceStart.current = windowed.start;
        setSlice(rows ?? []);
      }
    }).catch(() => {
      if (live) setSlice([]);
    });
    return () => {
      live = false;
    };
  }, [frames, fetchRange, total, windowed.start, windowed.visible]);

  useEffect(() => {
    if (total === 0 || windowed.selected < 0) {
      setPickedFrame(null);
      return;
    }
    if (frames) {
      return;
    }
    if (!getFrame) {
      const frame = sliceStart.current === windowed.start ? slice[localSelected] : undefined;
      setPickedFrame(frame ? { index: windowed.selected, frame } : null);
      return;
    }
    let live = true;
    getFrame(windowed.selected).then((f) => {
      if (live) setPickedFrame(f ? { index: windowed.selected, frame: f } : null);
    }).catch(() => {
      if (live) setPickedFrame(null);
    });
    return () => {
      live = false;
    };
  }, [frames, getFrame, localSelected, slice, total, windowed.selected, windowed.start]);

  useEffect(() => {
    if (total === 0 || windowed.selected < 0) {
      setHierarchy([]);
      return;
    }
    if (!getHierarchy) {
      setHierarchy([]);
      return;
    }
    let live = true;
    getHierarchy(windowed.selected).then((rows) => {
      if (live) setHierarchy(rows ?? []);
    }).catch(() => {
      if (live) setHierarchy([]);
    });
    return () => {
      live = false;
    };
  }, [getHierarchy, total, windowed.selected]);

  const onResizePointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    try {
      e.currentTarget.setPointerCapture(e.pointerId);
    } catch {
      // jsdom 可能没有 Pointer Capture
    }
    drag.current = { startY: e.clientY, startH: detailH };
  }, [detailH]);

  const onResizePointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!drag.current) return;
    setDetailH(clampDetailHeight(drag.current.startH + (drag.current.startY - e.clientY)));
  }, []);

  const onResizePointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!drag.current) return;
    drag.current = null;
    try {
      e.currentTarget.releasePointerCapture(e.pointerId);
    } catch {
      // capture 可能已释放
    }
    setDetailH((h) => {
      saveDetailHeight(h);
      return h;
    });
  }, []);

  const apply = useCallback((next: Viewport) => {
    setVp((cur) => ({
      ...clampViewport(next, total),
      visible: next.visible === windowed.visible ? cur.visible : next.visible,
    }));
  }, [total, windowed.visible]);

  const clampSeek = useCallback((i: number) => {
    apply(seekTo(windowed, total, i));
  }, [apply, total, windowed]);

  const onSelectLocal = useCallback((local: number) => {
    apply(selectInView(windowed, total, local));
  }, [apply, total, windowed]);

  const onWheelChart = useCallback((e: React.WheelEvent<HTMLElement>) => {
    if (total === 0) return;
    if (e.cancelable) e.preventDefault();
    const wrap = e.currentTarget;
    const rect = wrap.getBoundingClientRect();
    const local = localIndexFromX(e.clientX - rect.left, rect.width, windowed.visible);
    const anchor = windowed.start + local;
    if (wheelIntent(e.shiftKey, e.deltaX || 0, e.deltaY) === 'pan') {
      const dx = e.deltaX || 0;
      const delta = dx !== 0
        ? Math.sign(dx) * Math.max(1, Math.round(windowed.visible * 0.1))
        : wheelPanDelta(e.deltaY, windowed.visible);
      apply(panViewport(windowed, total, delta));
      return;
    }
    apply(zoomViewport(windowed, total, wheelZoomFactor(e.deltaY), anchor));
  }, [apply, total, windowed]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      if (windowed.selected < 0) return;
      if (e.key === 'ArrowLeft') {
        e.preventDefault();
        apply(seekTo(windowed, total, windowed.selected - 1));
      } else if (e.key === 'ArrowRight') {
        e.preventDefault();
        apply(seekTo(windowed, total, windowed.selected + 1));
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [apply, total, windowed]);

  const toggle = (id: ProfilerModuleId) => {
    setEnabled((cur) => {
      if (cur.includes(id)) {
        const next = cur.filter((x) => x !== id);
        return next.length === 0 ? cur : next;
      }
      return [...cur, id];
    });
    setActive(id);
  };

  const visibleMods = PROFILER_MODULES.filter((m) => enabled.includes(m.id));
  const chartLayoutRevision = visibleMods.map((m) => m.id).join('|');
  const canScroll = total > windowed.visible;
  const viewEnd = total === 0 ? 0 : Math.min(total - 1, windowed.start + windowed.visible - 1);

  return (
    <div className="profiler" data-testid="profiler-view">
      <ProfilerToolbar
        recording={recording}
        connected={connected}
        frame={picked}
        selectedIndex={total === 0 || windowed.selected < 0 ? -1 : windowed.selected}
        total={total}
        onClear={() => { setHeavyRange(null); onClear(); }}
        onPrev={() => apply(seekTo(windowed, total, windowed.selected - 1))}
        onNext={() => apply(seekTo(windowed, total, windowed.selected + 1))}
        onSeek={clampSeek}
        onCurrent={() => apply(goCurrent(windowed, total))}
        onToggleRecord={onToggleRecord}
        onPause={onPause}

        onImport={onImport}
        deepProfiling={deepProfiling}
        callStacks={callStacks}
        onDeepProfiling={onDeepProfiling}
        onCallStacks={onCallStacks}
      />
      <div
        className="sr-only"
        data-testid="profiler-viewport"
        data-start={windowed.start}
        data-visible={windowed.visible}
        data-selected={windowed.selected}
        data-follow={windowed.follow ? 'true' : 'false'}
      />
      <div className="profiler-modules" role="group" aria-label="Profiler 模块">
        {PROFILER_MODULES.map((m) => (
          <label key={m.id} className={`profiler-mod ${enabled.includes(m.id) ? 'is-on' : ''}`} data-testid={`profiler-module-${m.id}`}>
            <input
              type="checkbox"
              checked={enabled.includes(m.id)}
              onChange={() => toggle(m.id)}
              aria-label={m.label}
            />
            <span className="swatch" style={{ background: m.color }} />
            {m.label}
          </label>
        ))}
      </div>
      {getScreenshots && setScreenCapture && (
        <ScreenFilmstrip fetchStrip={getScreenshots} setCapture={setScreenCapture} connected={connected}
          start={windowed.start} count={Math.min(windowed.visible, Math.max(0, total - windowed.start))}
          selected={windowed.selected} onSelect={clampSeek} />
      )}
      <div
        className="profiler-charts"
        data-testid="profiler-charts"
        title="滚轮缩放，Shift+滚轮平移"
      >
        {visibleMods.map((m) => (
          <ProfilerChart
            key={m.id}
            module={m}
            frames={slice}
            selectedIndex={localSelected}
            active={active === m.id}
            onSelectFrame={onSelectLocal}
            frameRange={m.id === 'cpu' && active === 'cpu' && cpuRange ? {
              start: cpuRange.start - sliceStart.current, end: cpuRange.end - sliceStart.current,
            } : undefined}
            onSelectRange={m.id === 'cpu' && getHeavyProfile ? (range) => {
              if (sliceStart.current !== windowed.start) return;
              setHeavyRange({ start: windowed.start + range.start, end: windowed.start + range.end });
              setCpuTab('left-heavy');
              apply(selectInView(windowed, total, range.start));
            } : undefined}
            onActivate={() => setActive(m.id)}
            onWheel={onWheelChart}
            yAxisMax={m.id === 'cpu' || m.id === 'gpu' || m.id === 'rendering' ? yAxisMax[m.id] : undefined}
            onYAxisMaxChange={m.id === 'cpu' || m.id === 'gpu' || m.id === 'rendering'
              ? (max) => setYAxisMax((current) => ({ ...current, [m.id]: max }))
              : undefined}
            layoutRevision={chartLayoutRevision}
            fillAvailableHeight
          />
        ))}
      </div>
      <div className="profiler-scroll-wrap">
        <input
          type="range"
          className="profiler-scroll"
          aria-label="时间轴滚动"
          min={0}
          max={canScroll ? maxStart(total, windowed.visible) : 0}
          value={windowed.start}
          disabled={!canScroll}
          onChange={(e) => apply(setViewportStart(windowed, total, Number.parseInt(e.target.value, 10)))}
        />
        <span className="mono dim profiler-scroll-meta" data-testid="profiler-range">
          {total === 0 ? '无帧' : `${windowed.start + 1}–${viewEnd + 1} · ${windowed.visible}帧`}
        </span>
      </div>
      <div className="profiler-detail-wrap" style={{ height: detailH }} data-testid="profiler-detail-wrap">
        <div
          className="profiler-resize"
          role="separator"
          aria-orientation="horizontal"
          aria-label="调整详情面板高度"
          data-testid="profiler-resize"
          onPointerDown={onResizePointerDown}
          onPointerMove={onResizePointerMove}
          onPointerUp={onResizePointerUp}
          onPointerCancel={onResizePointerUp}
        />
        <ProfilerDetail moduleId={active} frame={picked} hierarchy={hierarchy} callStacks={callStacks}
          onTimelineRange={syncTimelineRange}
          getHeavyProfile={getHeavyProfile} heavyRange={validHeavyRange} onHeavyRange={setHeavyRange}
          cpuTab={cpuTab} onCpuTabChange={setCpuTab} totalFrames={total}
          getTimeline={getTimeline} timelineSelection={windowed.selected} onTimelineSeek={clampSeek}
          timelineFallback={total > 0 ? pickedFrame : null} refreshTimeline={recording && total - windowed.selected <= 120} />
      </div>
    </div>
  );
}
