import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { FrameData, SampleNode } from '../types';
import { formatMs } from '../lib/format';
import {
  BAR_H,
  RULER_H,
  THREAD_LABEL_W,
  barEnd,
  barY,
  clampWindow,
  fitWindow,
  hitTest,
  layoutHeight,
  layoutTimeline,
  msFromX,
  panDeltaFromPixels,
  panWindow,
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
}

const RULER = 'rgba(148, 163, 184, 0.75)';
const GRID = 'rgba(148, 163, 184, 0.16)';
const SELECT = 'rgba(248, 250, 252, 0.95)';
const LABEL_BG = '#12151c';
const PLOT_BG = '#0c0f14';
const THREAD_BG = 'rgba(26, 31, 41, 0.45)';

/**
 * Unity Profiler 2022 CPU Timeline：线程泳道 + 嵌套采样条。
 * 滚轮缩放（锚在指针），拖拽平移，单击选中，双击放大到该采样。
 */
export function CpuTimeline({ frame, hierarchy }: Props) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const layout = useMemo(() => layoutTimeline(hierarchy, frame), [hierarchy, frame]);
  const [win, setWin] = useState<TimeWindow>(() => fitWindow(layout.totalMs));
  const winRef = useRef(win);
  winRef.current = win;
  const [selected, setSelected] = useState<string>('');
  const [hover, setHover] = useState<{ bar: TimelineBar; x: number; y: number } | null>(null);
  const drag = useRef<{
    pointerId: number;
    startX: number;
    startWin: TimeWindow;
    moved: boolean;
  } | null>(null);

  useEffect(() => {
    setWin(fitWindow(layout.totalMs));
    setSelected('');
    setHover(null);
  }, [layout.totalMs, frame.frameIndex, hierarchy]);

  const selectedBar = useMemo(() => {
    for (const th of layout.threads) {
      const bar = th.bars.find((b) => b.id === selected);
      if (bar) return bar;
    }
    return null;
  }, [layout.threads, selected]);

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    const wrap = wrapRef.current;
    if (!canvas || !wrap) return;
    const dpr = window.devicePixelRatio || 1;
    const w = wrap.clientWidth;
    const contentH = Math.max(layoutHeight(layout.threads), wrap.clientHeight);
    if (w <= 0) return;
    canvas.width = Math.floor(w * dpr);
    canvas.height = Math.floor(contentH * dpr);
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
    layout.threads.forEach((th, ti) => {
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
      ctx.fillText(th.name, 8, top + h / 2);
      ctx.restore();

      const viewStart = cur.startMs;
      const viewEnd = cur.startMs + cur.spanMs;
      ctx.font = '10px Inter, Segoe UI, system-ui, sans-serif';
      for (const bar of th.bars) {
        const end = barEnd(bar);
        if (end < viewStart || bar.startMs > viewEnd) continue;
        const x0 = THREAD_LABEL_W + xFromMs(bar.startMs, plotW, cur);
        const x1 = THREAD_LABEL_W + xFromMs(end, plotW, cur);
        const bx = Math.max(THREAD_LABEL_W, x0);
        const bw = Math.max(1, Math.min(w, x1) - bx);
        const by = barY(top, bar.depth);
        ctx.fillStyle = bar.color;
        ctx.globalAlpha = bar.id === selected ? 1 : 0.92;
        ctx.fillRect(bx, by, bw, BAR_H);
        ctx.globalAlpha = 1;
        if (bar.id === selected) {
          ctx.strokeStyle = SELECT;
          ctx.lineWidth = 1.5;
          ctx.strokeRect(bx + 0.5, by + 0.5, Math.max(0, bw - 1), BAR_H - 1);
        }
        if (bw >= 28) {
          ctx.fillStyle = 'rgba(248, 250, 252, 0.92)';
          ctx.save();
          ctx.beginPath();
          ctx.rect(bx + 2, by, bw - 4, BAR_H);
          ctx.clip();
          ctx.fillText(bar.name, bx + 4, by + BAR_H / 2);
          ctx.restore();
        }
      }
      top += h + THREAD_GAP;
    });
  }, [layout.threads, selected]);

  useEffect(() => {
    draw();
    const wrap = wrapRef.current;
    if (!wrap) return;
    const ro = new ResizeObserver(draw);
    ro.observe(wrap);
    return () => ro.disconnect();
  }, [draw, win]);

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
      layout.threads,
      clientX - rect.left,
      clientY - rect.top + wrap.scrollTop,
      THREAD_LABEL_W,
      plotW(),
      winRef.current,
    );
  };

  const onWheel = (e: React.WheelEvent<HTMLDivElement>) => {
    if (e.cancelable) e.preventDefault();
    const wrap = wrapRef.current;
    if (!wrap) return;
    const rect = wrap.getBoundingClientRect();
    const x = e.clientX - rect.left - THREAD_LABEL_W;
    const anchor = msFromX(Math.max(0, x), plotW(), winRef.current);
    if (e.shiftKey || e.deltaX) {
      const dx = e.deltaX || e.deltaY;
      setWin((cur) => panWindow(cur, layout.totalMs, panDeltaFromPixels(-dx, plotW(), cur)));
      return;
    }
    setWin((cur) => zoomWindow(cur, layout.totalMs, wheelZoomFactor(e.deltaY), anchor));
  };

  return (
    <div className="cpu-timeline" data-testid="cpu-timeline">
      <div className="sr-only" data-testid="cpu-timeline-threads">
        {layout.threads.map((th) => (
          <span key={th.name}>{th.name}</span>
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
        className="cpu-timeline-plot"
        data-testid="cpu-timeline-plot"
        title="滚轮缩放，拖拽平移，双击放大采样"
        onWheel={onWheel}
        onPointerDown={(e) => {
          if (e.button !== 0 && e.button !== 1) return;
          (e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
          drag.current = {
            pointerId: e.pointerId,
            startX: e.clientX,
            startWin: winRef.current,
            moved: false,
          };
        }}
        onPointerMove={(e) => {
          const hit = pick(e.clientX, e.clientY);
          const wrap = wrapRef.current;
          if (wrap && hit) {
            const rect = wrap.getBoundingClientRect();
            setHover({ bar: hit, x: e.clientX - rect.left, y: e.clientY - rect.top + wrap.scrollTop });
          } else {
            setHover(null);
          }
          const d = drag.current;
          if (!d || d.pointerId !== e.pointerId) return;
          const dx = e.clientX - d.startX;
          if (Math.abs(dx) > 3) d.moved = true;
          if (!d.moved) return;
          setWin(panWindow(d.startWin, layout.totalMs, panDeltaFromPixels(dx, plotW(), d.startWin)));
        }}
        onPointerUp={(e) => {
          const d = drag.current;
          drag.current = null;
          try {
            e.currentTarget.releasePointerCapture?.(e.pointerId);
          } catch {
            // capture 可能已释放
          }
          if (d?.moved) return;
          const hit = pick(e.clientX, e.clientY);
          setSelected(hit?.id ?? '');
        }}
        onPointerCancel={() => {
          drag.current = null;
        }}
        onDoubleClick={(e) => {
          const hit = pick(e.clientX, e.clientY);
          if (!hit) {
            setWin(fitWindow(layout.totalMs));
            return;
          }
          setSelected(hit.id);
          setWin(zoomToBar(hit, layout.totalMs));
        }}
      >
        <canvas ref={canvasRef} />
        {hover && (
          <div
            className="cpu-timeline-tip"
            data-testid="cpu-timeline-tip"
            style={{ left: Math.min(hover.x + 12, (wrapRef.current?.clientWidth ?? 0) - 8), top: hover.y + 12 }}
          >
            <strong>{hover.bar.name}</strong>
            <span className="mono">{formatMs(hover.bar.durationMs)}</span>
          </div>
        )}
      </div>
      <div className="cpu-timeline-status" data-testid="cpu-timeline-status">
        {selectedBar ? (
          <>
            <span className="swatch" style={{ background: selectedBar.color }} />
            <span className="cpu-timeline-status-name" title={selectedBar.name}>{selectedBar.name}</span>
            <span className="mono dim">{formatMs(selectedBar.durationMs)}</span>
            <span className="mono dim">{formatMs(selectedBar.startMs)}–{formatMs(barEnd(selectedBar))}</span>
          </>
        ) : (
          <span className="dim">滚轮缩放 · 拖拽平移 · 单击选中 · 双击放大</span>
        )}
        <button
          type="button"
          className="btn ghost cpu-timeline-fit"
          aria-label="适应窗口"
          title="适应窗口"
          onClick={() => setWin(clampWindow(fitWindow(layout.totalMs), layout.totalMs))}
        >
          适应
        </button>
      </div>
    </div>
  );
}
