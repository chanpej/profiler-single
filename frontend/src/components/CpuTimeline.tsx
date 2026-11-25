import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { CpuThreadSamples, CpuTimelineFrame, FrameData, FrameRange, SampleNode } from '../types';
import { formatMs } from '../lib/format';
import {
  BAR_H,
  BAR_GAP,
  MIN_SPAN_MS,
  RULER_H,
  THREAD_LABEL_W,
  barEnd,
  barsInWindow,
  barY,
  clampWindow,
  fitWindow,
  hitTest,
  layoutHeight,
  layoutTimeline,
  layoutTimelineContext,
  msFromX,
  panDeltaFromPixels,
  panWindow,
  projectSample,
  threadHeight,
  THREAD_GAP,
  timeTicks,
  type TimelineBar,
  type TimeWindow,
  wheelZoomFactor,
  xFromMs,
  zoomToBar,
  zoomWindow,
} from '../lib/cpuTimeline';

interface Props {
  frame: FrameData;
  hierarchy: SampleNode[];
  threadSamples?: CpuThreadSamples[];
  context?: CpuTimelineFrame[];
  loadingContext?: boolean;
  canPageLeft?: boolean;
  selectionIndex?: number;
  onVisibleRange?: (range: FrameRange | null) => void;
  canPageRight?: boolean;
  onExtend?: (edge: CpuTimelineFrame, direction: -1 | 1) => void;
  onSelectNeighbor?: (frame: CpuTimelineFrame) => void;
}

const RULER = 'rgba(148, 163, 184, 0.75)';
const GRID = 'rgba(148, 163, 184, 0.16)';
const SELECT = 'rgba(248, 250, 252, 0.95)';
const LABEL_BG = '#12151c';
const PLOT_BG = '#0c0f14';
const THREAD_BG = 'rgba(26, 31, 41, 0.45)';
const EMPTY_CONTEXT: CpuTimelineFrame[] = [];
const labelStyleKey = 'upa.cpuTimeline.labelStyle';
type LabelStyle = 'split' | 'unity';

function readLabelStyle(): LabelStyle {
  try {
    return localStorage.getItem(labelStyleKey) === 'unity' ? 'unity' : 'split';
  } catch {
    return 'split';
  }
}

function drawUnityLabel(ctx: CanvasRenderingContext2D, bar: TimelineBar, left: number, right: number, top: number) {
  const width = right - left;
  if (width <= 4) return;
  const text = `${bar.name} (${formatMs(bar.durationMs)})`;
  const textWidth = ctx.measureText(text).width;
  const fade = Math.min(1, (width - 4) / Math.min(80, Math.max(1, textWidth)));
  ctx.save();
  ctx.globalAlpha *= fade;
  ctx.fillStyle = 'rgba(248, 250, 252, 0.92)';
  ctx.beginPath();
  ctx.rect(left, top, width, BAR_H);
  ctx.clip();
  ctx.fillText(text, left + Math.max(0, (width - textWidth) / 2), top + BAR_H / 2);
  ctx.restore();
}

/**
 * Unity Profiler 2022 CPU Timeline：线程泳道 + 嵌套采样条。
 * 滚轮缩放（锚在指针），中键拖拽平移，单击选中，双击放大到该采样。
 */
export function CpuTimeline({ frame, hierarchy, threadSamples, context = EMPTY_CONTEXT, loadingContext = false, onSelectNeighbor, onExtend, canPageLeft, canPageRight, selectionIndex = -1, onVisibleRange }: Props) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const currentFrame = context.find((candidate) => candidate.selected);
  const current = useMemo(() => {
    const layout = layoutTimeline(hierarchy, currentFrame ? null : frame, currentFrame?.threads ?? threadSamples);
    return currentFrame ? { ...layout, totalMs: Math.max(layout.totalMs, currentFrame.durationMs) } : layout;
  }, [hierarchy, frame, currentFrame, threadSamples]);
  const layout = useMemo(() => layoutTimelineContext(current, context), [current, context]);
  const fit = useMemo(() => currentFrame && currentFrame.durationMs > 0
    ? fitWindow(currentFrame.durationMs)
    : fitWindow(current.totalMs, current.startMs), [currentFrame?.durationMs, current.startMs, current.totalMs]);
  const [win, setWin] = useState<TimeWindow>(() => fit);
  const winRef = useRef(win);
  winRef.current = win;
  const [selected, setSelected] = useState<string>('');
  const [labelStyle, setLabelStyle] = useState<LabelStyle>(readLabelStyle);
  const [visibleDepths, setVisibleDepths] = useState<Record<string, number>>({});
  const [pointer, setPointer] = useState<{ clientX: number; clientY: number } | null>(null);
  const [panning, setPanning] = useState(false);
  const resize = useRef<{ thread: string; startY: number; depth: number } | null>(null);
  const rangeDrag = useRef<{ pointerId: number; startX: number; startWin: TimeWindow } | null>(null);
  const drag = useRef<{
    pointerId: number;
    startX: number;
    startY: number;
    scrollTop: number;
    mode: 'select' | 'pan' | 'zoom';
    startWin: TimeWindow;
    moved: boolean;
  } | null>(null);
  const visibleThreads = useMemo(() => layout.threads.map((thread) => ({
    ...thread, maxDepth: Math.min(thread.maxDepth, visibleDepths[thread.id] ?? thread.maxDepth),
  })), [layout.threads, visibleDepths]);
  const laneControls = useMemo(() => {
    let top = RULER_H;
    return visibleThreads.map((thread, index) => {
      const bottom = top + threadHeight(thread.maxDepth);
      const lane = { thread, top, bottom, maxDepth: layout.threads[index].maxDepth };
      top = bottom + THREAD_GAP;
      return lane;
    });
  }, [visibleThreads, layout.threads]);
  const previousLayout = useRef({ frameIndex: frame.frameIndex, fit });
  const hasSwitchedFrame = useRef(false);
  const pendingNeighbor = useRef<CpuTimelineFrame | null>(null);
  const desiredPan = useRef<TimeWindow | null>(null);
  const requestedEdges = useRef(new Set<string>());
  const pagePan = useCallback((target: TimeWindow) => {
    const clamped = clampWindow(target, layout.totalMs, layout.startMs);
    setWin(clamped);
    const edge = target.startMs < clamped.startMs && canPageLeft ? context[0]
      : target.startMs > clamped.startMs && canPageRight ? context[context.length - 1] : undefined;
    if (edge && onExtend) {
      desiredPan.current = target;
      const direction = target.startMs < clamped.startMs ? -1 : 1;
      const key = `${direction}:${edge.frameIndex}`;
      if (!requestedEdges.current.has(key)) {
        requestedEdges.current.add(key);
        onExtend(edge, direction);
      }
    } else {
      desiredPan.current = null;
    }
  }, [layout.totalMs, layout.startMs, context, canPageLeft, canPageRight, onExtend]);

  useLayoutEffect(() => {
    const previous = previousLayout.current;
    previousLayout.current = { frameIndex: frame.frameIndex, fit };
    if (previous.frameIndex !== frame.frameIndex) {
      hasSwitchedFrame.current = true;
      const neighbor = pendingNeighbor.current;
      pendingNeighbor.current = null;
      if (neighbor?.frameIndex === frame.frameIndex) {
        setWin((cur) => ({ ...cur, startMs: cur.startMs - neighbor.offsetMs }));
      }
      setSelected('');
      setPointer(null);
      desiredPan.current = null;
      requestedEdges.current.clear();
      setPanning(false);
      drag.current = null;
      rangeDrag.current = null;
    }
    if (!loadingContext) {
      const followInitialFit = !hasSwitchedFrame.current;
      setWin((cur) => followInitialFit && cur.startMs === previous.fit.startMs && cur.spanMs === previous.fit.spanMs
        ? fit
        : clampWindow(cur, layout.totalMs, layout.startMs));
    }
  }, [layout.totalMs, layout.startMs, frame.frameIndex, fit, loadingContext]);

  useEffect(() => {
    if (!loadingContext && !pendingNeighbor.current && desiredPan.current) pagePan(desiredPan.current);
  }, [loadingContext, pagePan]);

  const selectedBar = useMemo(() => {
    if (!selected) return null;
    for (const th of current.threads) {
      const bar = th.bars.find((b) => b.id === selected);
      if (bar) return bar;
    }
    return null;
  }, [current.threads, selected]);

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    const wrap = wrapRef.current;
    if (!canvas || !wrap) return;
    const dpr = window.devicePixelRatio || 1;
    const w = wrap.clientWidth;
    const contentH = Math.max(layoutHeight(visibleThreads), wrap.clientHeight);
    if (w <= 0) return;
    const pixelWidth = Math.floor(w * dpr);
    const pixelHeight = Math.floor(contentH * dpr);
    if (canvas.width !== pixelWidth) canvas.width = pixelWidth;
    if (canvas.height !== pixelHeight) canvas.height = pixelHeight;
    canvas.style.width = `${w}px`;
    canvas.style.height = `${contentH}px`;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, contentH);

    const plotW = Math.max(1, w - THREAD_LABEL_W);
    const cur = winRef.current;

    ctx.fillStyle = PLOT_BG;
    ctx.fillRect(THREAD_LABEL_W, 0, plotW, contentH);
    ctx.fillStyle = LABEL_BG;
    ctx.fillRect(0, 0, THREAD_LABEL_W, contentH);

    ctx.strokeStyle = GRID;
    ctx.lineWidth = 1;
    const ticks = timeTicks(cur, plotW);
    ctx.font = '10px Menlo, Consolas, monospace';
    ctx.fillStyle = RULER;
    ctx.textBaseline = 'middle';
    for (const tick of ticks) {
      const x = THREAD_LABEL_W + xFromMs(tick.ms, plotW, cur);
      if (x < THREAD_LABEL_W - 1 || x > w) continue;
      ctx.beginPath();
      ctx.moveTo(x + 0.5, RULER_H);
      ctx.lineTo(x + 0.5, contentH);
      ctx.stroke();
      ctx.fillText(tick.label, x + 4, RULER_H / 2);
    }
    ctx.fillStyle = LABEL_BG;
    ctx.fillRect(0, 0, THREAD_LABEL_W, RULER_H);
    ctx.strokeStyle = 'rgba(35, 41, 54, 1)';
    ctx.beginPath();
    ctx.moveTo(0, RULER_H + 0.5);
    ctx.lineTo(w, RULER_H + 0.5);
    ctx.moveTo(THREAD_LABEL_W + 0.5, 0);
    ctx.lineTo(THREAD_LABEL_W + 0.5, contentH);
    ctx.stroke();

    let top = RULER_H;
    ctx.textBaseline = 'middle';
    visibleThreads.forEach((th, ti) => {
      const h = threadHeight(th.maxDepth);
      if (ti % 2 === 0) {
        ctx.fillStyle = THREAD_BG;
        ctx.fillRect(THREAD_LABEL_W, top, plotW, h);
      }
      ctx.fillStyle = '#94a3b8';
      ctx.font = '11px Inter, Segoe UI, system-ui, sans-serif';
      ctx.save();
      ctx.beginPath();
      ctx.rect(4, top, THREAD_LABEL_W - 8, h);
      ctx.clip();
      ctx.fillText(th.name, 22, barY(top, 0) + BAR_H / 2);
      ctx.restore();

      ctx.font = '10px Inter, Segoe UI, system-ui, sans-serif';
      for (const bar of barsInWindow(th, cur)) {
        if (bar.depth > th.maxDepth) continue;
        const bounds = projectSample(bar, plotW, cur);
        if (!bounds) continue;
        const bx = THREAD_LABEL_W + bounds.left;
        const bw = bounds.right - bounds.left;
        const by = barY(top, bar.depth);
        ctx.fillStyle = bar.color;
        ctx.globalAlpha = bar.ghost ? 0.3 : 1;
        ctx.fillRect(bx, by, bw, BAR_H);
        if (bar.id === selected) {
          ctx.strokeStyle = SELECT;
          ctx.lineWidth = 1.5;
          ctx.strokeRect(bx + 0.5, by + 0.5, Math.max(0, bw - 1), BAR_H - 1);
        }
        if (bw >= 12 && (bounds.continuesLeft || bounds.continuesRight)) {
          ctx.strokeStyle = 'rgba(248, 250, 252, 0.75)';
          ctx.lineWidth = 1;
          const middle = by + BAR_H / 2;
          const drawContinuation = (edge: number, direction: number) => {
            ctx.beginPath();
            ctx.moveTo(edge + 4 * direction, middle - 3);
            ctx.lineTo(edge + direction, middle);
            ctx.lineTo(edge + 4 * direction, middle + 3);
            ctx.stroke();
          };
          if (bounds.continuesLeft) drawContinuation(bx, 1);
          if (bounds.continuesRight) drawContinuation(bx + bw, -1);
        }
        if (labelStyle === 'unity') {
          drawUnityLabel(ctx, bar, bx + (bounds.continuesLeft ? 8 : 3), bx + bw - (bounds.continuesRight ? 8 : 3), by);
        } else if (bw >= 18) {
          ctx.fillStyle = 'rgba(248, 250, 252, 0.92)';
          const labelStart = bx + (bounds.continuesLeft ? 8 : 4);
          const labelEnd = bx + bw - (bounds.continuesRight ? 8 : 2);
          const duration = formatMs(bar.durationMs);
          const durationStart = labelEnd - ctx.measureText(duration).width;
          if (durationStart >= labelStart) {
            ctx.fillText(duration, durationStart, by + BAR_H / 2);
          }
          const nameEnd = durationStart - 6;
          if (nameEnd - labelStart >= 14) {
            ctx.save();
            ctx.beginPath();
            ctx.rect(labelStart, by, nameEnd - labelStart, BAR_H);
            ctx.clip();
            ctx.fillText(bar.name, labelStart, by + BAR_H / 2);
            ctx.restore();
          }
        }
        ctx.globalAlpha = 1;
      }
      top += h + THREAD_GAP;
    });

    if (context.length > 1) {
      ctx.font = '10px Menlo, Consolas, monospace';
      for (const boundary of context) {
        const x = THREAD_LABEL_W + xFromMs(boundary.offsetMs, plotW, cur);
        if (x < THREAD_LABEL_W || x >= w) continue;
        ctx.strokeStyle = boundary.selected ? 'rgba(148, 163, 184, 0.8)' : 'rgba(148, 163, 184, 0.4)';
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(x + 0.5, RULER_H);
        ctx.lineTo(x + 0.5, contentH);
        ctx.stroke();
        if (x > THREAD_LABEL_W && w - x >= 55) {
          ctx.fillStyle = PLOT_BG;
          ctx.fillRect(x + 2, 1, 52, RULER_H - 2);
          ctx.fillStyle = '#cbd5e1';
          ctx.fillText(`#${boundary.frameIndex}`, x + 5, RULER_H / 2);
        }
      }
    }
  }, [context, visibleThreads, selected, labelStyle]);

  useLayoutEffect(() => {
    draw();
  }, [draw, win]);

  useEffect(() => {
    const wrap = wrapRef.current;
    if (!wrap) return;
    const ro = new ResizeObserver(draw);
    ro.observe(wrap);
    return () => ro.disconnect();
  }, [draw]);

  const plotW = () => {
    const wrap = wrapRef.current;
    if (!wrap) return 1;
    return Math.max(1, wrap.clientWidth - THREAD_LABEL_W);
  };

  const pick = (clientX: number, clientY: number) => {
    const wrap = wrapRef.current;
    if (!wrap) return null;
    const rect = wrap.getBoundingClientRect();
    return hitTest(
      visibleThreads,
      clientX - rect.left,
      clientY - rect.top + wrap.scrollTop,
      THREAD_LABEL_W,
      plotW(),
      winRef.current,
    );
  };

  const hover = (() => {
    const wrap = wrapRef.current;
    if (!pointer || !wrap) return null;
    const rect = wrap.getBoundingClientRect();
    const x = pointer.clientX - rect.left;
    const y = pointer.clientY - rect.top;
    if (y < 0 || y >= wrap.clientHeight) return null;
    const bar = pick(pointer.clientX, pointer.clientY);
    if (!bar || bar.id === selected) return null;
    const width = Math.min(280, wrap.clientWidth);
    return {
      bar,
      width,
      left: Math.max(0, Math.min(x + 12, wrap.clientWidth - width)),
      top: wrap.scrollTop + (y + 40 <= wrap.clientHeight ? y + 12 : Math.max(0, y - 36)),
    };
  })();

  const pinned = (() => {
    const wrap = wrapRef.current;
    if (!selectedBar || !wrap) return null;
    const lane = laneControls.find(({ thread }) => thread.bars.some((bar) => bar.id === selectedBar.id));
    if (!lane || selectedBar.depth > lane.thread.maxDepth) return null;
    const bounds = projectSample(selectedBar, plotW(), win);
    if (!bounds) return null;
    const width = Math.min(280, wrap.clientWidth);
    const y = barY(lane.top, selectedBar.depth);
    const below = y + BAR_H + 4;
    return {
      width,
      left: Math.max(0, Math.min(THREAD_LABEL_W + bounds.left, wrap.clientWidth - width)),
      top: below + 26 <= laneControls[laneControls.length - 1].bottom ? below : Math.max(0, y - 28),
    };
  })();

  const neighborAt = (clientX: number) => {
    const wrap = wrapRef.current;
    if (!wrap || !onSelectNeighbor) return undefined;
    const plotX = clientX - wrap.getBoundingClientRect().left - THREAD_LABEL_W;
    const width = plotW();
    if (plotX < 0 || plotX >= width) return undefined;
    const time = msFromX(plotX, width, winRef.current);
    return context.find((candidate) => !candidate.selected && time >= candidate.offsetMs && time < candidate.offsetMs + candidate.durationMs);
  };

  useEffect(() => {
    const wrap = wrapRef.current;
    if (!wrap) return;
    const onWheel = (event: WheelEvent) => {
      const x = event.clientX - wrap.getBoundingClientRect().left - THREAD_LABEL_W;
      if (x < 0 && !event.shiftKey && !event.deltaX) return;
      event.preventDefault();
      const width = Math.max(1, wrap.clientWidth - THREAD_LABEL_W);
      if (event.shiftKey || event.deltaX) {
        const delta = event.deltaX || event.deltaY;
        setWin((cur) => panWindow(cur, layout.totalMs, panDeltaFromPixels(-delta, width, cur), layout.startMs));
        return;
      }
      const factor = wheelZoomFactor(event.deltaY);
      setWin((cur) => zoomWindow(cur, layout.totalMs, factor, msFromX(x, width, cur), layout.startMs));
    };
    wrap.addEventListener('wheel', onWheel, { passive: false });
    return () => wrap.removeEventListener('wheel', onWheel);
  }, [layout.startMs, layout.totalMs]);

  const focusSample = (bar: TimelineBar | null) => {
    setWin(bar ? zoomToBar(bar, layout.totalMs, layout.startMs) : fit);
  };

  const changeLaneDepth = (id: string, depth: number) => {
    setVisibleDepths((current) => ({ ...current, [id]: depth }));
  };

  const visibleEnd = win.startMs + win.spanMs;
  const visibleFrames = useMemo(() => {
    if (!onVisibleRange || selectionIndex < 0 || loadingContext) return null;
    if (!context.length) return { start: selectionIndex, end: selectionIndex };
    const center = context.findIndex((row) => row.selected);
    if (center < 0) return null;
    let first = -1;
    let last = -1;
    for (let position = 0; position < context.length; position++) {
      const row = context[position];
      if (row.offsetMs >= visibleEnd) break;
      if (row.offsetMs + row.durationMs <= win.startMs) continue;
      if (first < 0) first = position;
      last = position;
    }
    return first < 0 ? null : { start: selectionIndex + first - center, end: selectionIndex + last - center };
  }, [context, selectionIndex, loadingContext, win.startMs, visibleEnd, onVisibleRange]);
  const rangeStart = visibleFrames?.start;
  const rangeEnd = visibleFrames?.end;
  useEffect(() => {
    onVisibleRange?.(rangeStart === undefined || rangeEnd === undefined ? null : { start: rangeStart, end: rangeEnd });
  }, [rangeStart, rangeEnd, onVisibleRange]);
  useEffect(() => () => onVisibleRange?.(null), [onVisibleRange]);
  const maxTime = layout.startMs + fitWindow(layout.totalMs, layout.startMs).spanMs;

  return (
    <div className="cpu-timeline" data-testid="cpu-timeline">
      <div className="sr-only" data-testid="cpu-timeline-threads">
        {layout.threads.map((th) => (
          <span key={th.id}>{th.name}</span>
        ))}
      </div>
      <div
        className="sr-only"
        data-testid="cpu-timeline-window"
        data-start={win.startMs}
        data-span={win.spanMs}
        data-selected={selected}
      />
      <div
        ref={wrapRef}
        className={panning ? 'cpu-timeline-plot is-panning' : 'cpu-timeline-plot'}
        data-testid="cpu-timeline-plot"
        tabIndex={0}
        onKeyDown={(event) => {
          const key = event.key.toLowerCase();
          if (key !== 'a' && key !== 'f') return;
          event.preventDefault();
          event.stopPropagation();
          focusSample(key === 'f' ? selectedBar : null);
        }}
        onContextMenu={(event) => { if (event.altKey) event.preventDefault(); }}
        onPointerDown={(e) => {
          if (drag.current) return;
          if (e.button !== 0 && e.button !== 1 && !(e.altKey && e.button === 2)) return;
          if (e.button === 1) {
            requestedEdges.current.clear();
            e.preventDefault();
            setPanning(true);
            setPointer(null);
          }
          e.currentTarget.focus({ preventScroll: true });
          (e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
          drag.current = {
            pointerId: e.pointerId,
            startX: e.clientX,
            startY: e.clientY,
            scrollTop: e.currentTarget.scrollTop,
            mode: e.button === 2 ? 'zoom' : e.button === 1 ? 'pan' : 'select',
            startWin: winRef.current,
            moved: false,
          };
        }}
        onPointerMove={(e) => {
          const d = drag.current;
          if (!d) {
            setPointer({ clientX: e.clientX, clientY: e.clientY });
            return;
          }
          if (d.pointerId !== e.pointerId) return;
          const dx = e.clientX - d.startX;
          const dy = e.clientY - d.startY;
          if (Math.abs(dx) > 3 || Math.abs(dy) > 3) d.moved = true;
          if (!d.moved) return;
          setPointer(null);
          if (d.mode === 'zoom') {
            const rect = e.currentTarget.getBoundingClientRect();
            const anchor = msFromX(Math.max(0, d.startX - rect.left - THREAD_LABEL_W), plotW(), d.startWin);
            setWin(zoomWindow(d.startWin, layout.totalMs, Math.exp(-dx / 160), anchor, layout.startMs));
            return;
          }
          if (d.mode !== 'pan') return;
          e.currentTarget.scrollTop = d.scrollTop - dy;
          const target = { ...d.startWin, startMs: d.startWin.startMs + panDeltaFromPixels(dx, plotW(), d.startWin) };
          if (pendingNeighbor.current || loadingContext) desiredPan.current = target;
          else pagePan(target);
        }}
        onPointerUp={(e) => {
          const d = drag.current;
          if (!d || d.pointerId !== e.pointerId) return;
          drag.current = null;
          desiredPan.current = null;
          setPanning(false);
          try {
            e.currentTarget.releasePointerCapture?.(e.pointerId);
          } catch {
            // capture 可能已释放
          }
          if (d.moved || d.mode !== 'select') return;
          if (Math.abs(e.clientX - d.startX) > 3 || Math.abs(e.clientY - d.startY) > 3) return;
          setPointer({ clientX: e.clientX, clientY: e.clientY });
          const neighbor = neighborAt(e.clientX);
          if (neighbor) {
            pendingNeighbor.current = neighbor;
            onSelectNeighbor?.(neighbor);
            return;
          }
          const hit = pick(e.clientX, e.clientY);
          setSelected(hit?.id ?? '');
        }}
        onPointerCancel={() => {
          desiredPan.current = null;
          drag.current = null;
          setPanning(false);
          setPointer(null);
        }}
        onLostPointerCapture={(event) => {
          if (drag.current?.pointerId !== event.pointerId) return;
          desiredPan.current = null;
          drag.current = null;
          setPanning(false);
        }}
        onPointerLeave={() => setPointer(null)}
        onScroll={() => setPointer(null)}
        onDoubleClick={(e) => {
          if (neighborAt(e.clientX)) return;
          const hit = pick(e.clientX, e.clientY);
          if (!hit) return;
          setSelected(hit.id);
          focusSample(hit);
        }}
      >
        <canvas ref={canvasRef} />
        {laneControls.map(({ thread, top, bottom, maxDepth }) => maxDepth > 0 && (
          <div key={thread.id}>
            <button type="button" className="cpu-timeline-fold" style={{ top: top + 3 }}
              aria-label={`${thread.name}采样层级`} aria-expanded={thread.maxDepth > 0}
              onPointerDown={(event) => { if (event.button === 0) event.stopPropagation(); }}
              onPointerUp={(event) => { if (event.button === 0) event.stopPropagation(); }}
              onDoubleClick={(event) => event.stopPropagation()}
              onClick={() => changeLaneDepth(thread.id, thread.maxDepth > 0 ? 0 : maxDepth)}>
              {thread.maxDepth > 0 ? '▾' : '▸'}
            </button>
            <div className="cpu-timeline-lane-grip" style={{ top: bottom - 2 }}
              onPointerDown={(event) => {
                if (event.button !== 0) return;
                event.stopPropagation();
                event.currentTarget.setPointerCapture?.(event.pointerId);
                resize.current = { thread: thread.id, startY: event.clientY, depth: thread.maxDepth };
              }}
              onPointerMove={(event) => {
                if (resize.current?.thread !== thread.id) return;
                const depth = resize.current.depth + Math.round((event.clientY - resize.current.startY) / (BAR_H + BAR_GAP));
                changeLaneDepth(thread.id, Math.max(0, Math.min(maxDepth, depth)));
              }}
              onPointerUp={(event) => {
                if (resize.current?.thread !== thread.id) return;
                event.stopPropagation();
                resize.current = null;
              }}
              onPointerCancel={() => { resize.current = null; }}
              onDoubleClick={(event) => { event.stopPropagation(); changeLaneDepth(thread.id, maxDepth); }} />
          </div>
        ))}
        {pinned && selectedBar && (
          <div className="cpu-timeline-tip" data-testid="cpu-timeline-pinned-tip"
            style={{ left: pinned.left, top: pinned.top, maxWidth: pinned.width }}>
            <strong>{selectedBar.name}</strong>
            <span className="mono">{formatMs(selectedBar.durationMs)}</span>
          </div>
        )}
        {hover && (
          <div
            className="cpu-timeline-tip"
            data-testid="cpu-timeline-tip"
            role="tooltip"
            style={{ left: hover.left, top: hover.top, maxWidth: hover.width }}
          >
            <strong>{hover.bar.name}</strong>
            <span className="mono">{formatMs(hover.bar.durationMs)}</span>
          </div>
        )}
      </div>
      {layout.threads.length > 0 && (
        <div className="cpu-timeline-range" aria-label="Timeline 可见范围">
          <div
            className="cpu-timeline-range-window"
            data-testid="cpu-timeline-range-window"
            style={{
              left: `${100 * (win.startMs - layout.startMs) / (maxTime - layout.startMs)}%`,
              right: `${100 * (maxTime - visibleEnd) / (maxTime - layout.startMs)}%`,
            }}
            onPointerDown={(event) => {
              if (event.button !== 0) return;
              event.currentTarget.setPointerCapture?.(event.pointerId);
              rangeDrag.current = { pointerId: event.pointerId, startX: event.clientX, startWin: winRef.current };
            }}
            onPointerMove={(event) => {
              const active = rangeDrag.current;
              const width = event.currentTarget.parentElement?.clientWidth ?? 0;
              if (!active || active.pointerId !== event.pointerId || width === 0) return;
              const delta = (event.clientX - active.startX) * (maxTime - layout.startMs) / width;
              setWin(panWindow(active.startWin, layout.totalMs, delta, layout.startMs));
            }}
            onLostPointerCapture={() => { rangeDrag.current = null; }}
            onPointerUp={() => { rangeDrag.current = null; }}
            onPointerCancel={() => { rangeDrag.current = null; }}
          />
          <input type="range" aria-label="可见范围起点" min={layout.startMs} max={maxTime} step="any" value={win.startMs}
            onChange={(event) => {
              const startMs = Math.min(Number(event.target.value), visibleEnd - MIN_SPAN_MS);
              setWin({ startMs, spanMs: visibleEnd - startMs });
            }} />
          <input type="range" aria-label="可见范围终点" min={layout.startMs} max={maxTime} step="any" value={visibleEnd}
            onChange={(event) => setWin({ ...win, spanMs: Math.max(MIN_SPAN_MS, Number(event.target.value) - win.startMs) })} />
        </div>
      )}
      {selectedBar && (
        <>
          <div className="cpu-timeline-status" data-testid="cpu-timeline-status">
            <span className="swatch" style={{ background: selectedBar.color }} />
            <span className="cpu-timeline-status-name" title={selectedBar.name}>{selectedBar.name}</span>
            <span className="mono dim">{formatMs(selectedBar.durationMs)}</span>
            <span className="mono dim">{formatMs(selectedBar.startMs)}–{formatMs(barEnd(selectedBar))}</span>
            {selectedBar.selfMs !== undefined && <span className="mono dim">自身 {formatMs(selectedBar.selfMs)} · {selectedBar.calls} 次</span>}
          </div>
          <section className="cpu-sample-stack" aria-label="采样调用栈">
            <code title={selectedBar.path}>{selectedBar.samplePath?.join(' › ') ?? selectedBar.path}</code>
            {selectedBar.callstack?.length ? (
              <pre>{selectedBar.callstack.join('\n')}</pre>
            ) : <small className="dim">该采样没有上报符号调用栈</small>}
          </section>
        </>
      )}
      <div className="cpu-timeline-status" data-testid="cpu-timeline-footer">
        <span className="dim">{layout.threads.length > 0
          ? '左键选中采样或切换灰色帧 · 中键拖拽平移 · 滚轮缩放 · 双击放大 · A 适应整帧 · F 聚焦采样'
          : '此帧暂无带时间戳的 CPU 采样'}</span>
        <label className="cpu-timeline-style-control">
          文字样式
          <select className="cpu-timeline-label-style" value={labelStyle}
            onChange={(event) => {
              const next = event.target.value === 'unity' ? 'unity' : 'split';
              setLabelStyle(next);
              try { localStorage.setItem(labelStyleKey, next); } catch { return; }
            }}>
            <option value="split">耗时分列</option>
            <option value="unity">Unity 合并显示</option>
          </select>
        </label>
        <button
          type="button"
          className="btn ghost cpu-timeline-fit"
          aria-label="适应窗口"
          title="适应窗口"
          onClick={() => focusSample(null)}
        >
          适应
        </button>
      </div>
    </div>
  );
}
