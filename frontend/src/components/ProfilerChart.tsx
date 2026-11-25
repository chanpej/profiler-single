import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { FrameData, FrameRange } from '../types';
import { adjustCpuChartMax, chartMax, DEFAULT_CPU_CHART_MAX_Y, extractModuleSeries, formatChartAxisValue, memoryMetricAvailable, type ModuleDef } from '../lib/profiler';
import { formatProfilerValue } from '../lib/format';
import { drawCpuAreas, moveChartSeries } from '../lib/cpuChart';

interface Props {
  module: ModuleDef;
  frames: FrameData[];
  selectedIndex: number;
  active: boolean;
  height?: number;
  onSelectFrame: (index: number) => void;
  pan?: { start: number; visible: number; move: (start: number) => void };
  frameRange?: FrameRange;
  onSelectRange?: (range: FrameRange) => void;
  onActivate: () => void;
  /** 仅激活模块的左侧信息区显示此入口。 */
  onWheel?: (e: WheelEvent, plot: HTMLElement) => void;
  /** 可调纵轴的手动上限。 */
  yAxisMax?: number;
  onYAxisMaxChange?: (max: number | undefined) => void;
  /** 轨道显隐会改变 Canvas 可用空间，版本变更时必须重新测量绘制。 */
  layoutRevision?: string;
  /** 轨道应随可用区域变化填满自己的分配空间。 */
  fillAvailableHeight?: boolean;
}

const GRID = 'rgba(148, 163, 184, 0.14)';
const SELECT = 'rgba(226, 232, 240, 0.95)';
const LABEL = 'rgba(148, 163, 184, 0.75)';
/** 每条轨道共享的图内纵轴宽度，保证所有帧的 X 坐标对齐。 */
const Y_AXIS_WIDTH = 42;
const OVERFLOW = '#ff3158';

function indexFromClick(clientX: number, left: number, width: number, count: number): number {
  if (count <= 0) return 0;
  const x = Math.min(Math.max(clientX - left, 0), width);
  return Math.min(count - 1, Math.floor((x / width) * count));
}

type MemoryMetricId = Parameters<typeof memoryMetricAvailable>[1];

function memoryMetricForSeries(id: string): MemoryMetricId | null {
  const metrics: Record<string, MemoryMetricId> = {
    total: 'totalUsed',
    texture: 'texture',
    mesh: 'mesh',
    materialCount: 'materialCount',
    objectCount: 'objectCount',
    gc: 'gcUsed',
    audio: 'audioUsed',
  };
  return metrics[id] ?? null;
}

/**
 * Unity Profiler 风格模块图：左侧图例（含当前帧数值）+ 右侧 Canvas 堆叠/叠加柱。
 * 点击或拖拽图表选帧，点击整行激活该模块的详情面板。
 */
export function ProfilerChart({
  module, frames, selectedIndex, active, height, onSelectFrame, pan, frameRange, onSelectRange, onActivate, onWheel,
  yAxisMax, onYAxisMaxChange, layoutRevision, fillAvailableHeight = false,
}: Props) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const middleDrag = useRef<{ id: number; x: number; start: number; framesPerPixel: number } | null>(null);
  const dragging = useRef(false);
  const [rangeDraft, setRangeDraft] = useState<{ anchor: number; end: number } | null>(null);
  const rangeStart = rangeDraft ? Math.min(rangeDraft.anchor, rangeDraft.end) : frameRange?.start;
  const rangeEnd = rangeDraft ? Math.max(rangeDraft.anchor, rangeDraft.end) : frameRange?.end;
  const axisDrag = useRef<{ lastY: number } | null>(null);
  const series = useMemo(() => extractModuleSeries(module, frames), [module, frames]);
  const [seriesOrder, setSeriesOrder] = useState(() => module.categories.map((category) => category.id));
  const [hiddenSeries, setHiddenSeries] = useState<string[]>([]);
  const draggedSeries = useRef<string | null>(null);
  const selectedFrame = frames[selectedIndex];
  const isMemory = module.id === 'memory';
  const isCpu = module.id === 'cpu';
  useEffect(() => {
    const plot = wrapRef.current;
    if (!plot || !onWheel) return;
    const wheel = (e: WheelEvent) => {
      if (!e.ctrlKey && !(e.shiftKey && !isCpu)) return;
      if (!isCpu && e.target instanceof Element && e.target.closest('.profiler-y-axis-control')) return;
      if (e.cancelable) e.preventDefault();
      onWheel(e, plot);
    };
    plot.addEventListener('wheel', wheel, { passive: false });
    return () => plot.removeEventListener('wheel', wheel);
  }, [isCpu, onWheel]);
  const legendSeries = useMemo(() => isCpu
    ? [...series].sort((first, second) => seriesOrder.indexOf(first.id) - seriesOrder.indexOf(second.id))
    : series, [isCpu, series, seriesOrder]);
  const plottedSeries = useMemo(() => isCpu
    ? legendSeries.filter((item) => !hiddenSeries.includes(item.id))
    : series, [hiddenSeries, isCpu, legendSeries, series]);
  const autoMaxY = useMemo(() => chartMax(module, plottedSeries), [module, plottedSeries]);
  const supportsAdjustableYAxis = isCpu || module.id === 'gpu' || module.id === 'rendering';
  const maxY = yAxisMax ?? autoMaxY;
  const yAxisMaxRef = useRef(maxY);
  const supportsOverflowIndicator = supportsAdjustableYAxis;
  const chartHeight = height ?? (isMemory ? 170 : 88);
  const overflowingFrames = useMemo(() => {
    if (!supportsOverflowIndicator) return 0;
    return frames.reduce((count, _, i) => {
      const value = module.stacked
        ? plottedSeries.reduce((sum, s) => sum + Math.max(0, s.values[i] || 0), 0)
        : plottedSeries.reduce((peak, s) => Math.max(peak, s.values[i] || 0), 0);
      return count + (value > maxY ? 1 : 0);
    }, 0);
  }, [frames, maxY, module.stacked, plottedSeries, supportsOverflowIndicator]);
  const headline = selectedFrame && isMemory
    ? (memoryMetricAvailable(selectedFrame, 'totalUsed')
      ? module.headline(selectedFrame)
      : (memoryMetricAvailable(selectedFrame, 'audioUsed')
        ? { value: selectedFrame.audioUsedMB, unit: 'mb' as const }
        : null))
    : (selectedFrame ? module.headline(selectedFrame) : null);

  useLayoutEffect(() => {
    yAxisMaxRef.current = maxY;
  }, [maxY]);

  useEffect(() => {
    const canvas = canvasRef.current;
    const wrap = wrapRef.current;
    if (!canvas || !wrap) return;

    const draw = () => {
      const dpr = window.devicePixelRatio || 1;
      const w = wrap.clientWidth;
      // 始终使用当前 Module 自身轨道的高度，不使用图表总容器或相邻轨道的高度。
      // 因而增减可见 Module 后，Canvas 会随该轨道的新分配空间填满并重绘。
      const trackHeight = wrap.parentElement?.clientHeight ?? wrap.clientHeight;
      const h = fillAvailableHeight ? Math.max(chartHeight, trackHeight) : chartHeight;
      if (w === 0) return;
      canvas.width = Math.floor(w * dpr);
      canvas.height = Math.floor(h * dpr);
      canvas.style.width = `${w}px`;
      canvas.style.height = `${h}px`;
      const ctx = canvas.getContext('2d');
      if (!ctx) return;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, w, h);

      const padTop = 4;
      const padBottom = 4;
      const plotH = h - padTop - padBottom;
      const axisW = Y_AXIS_WIDTH;
      const plotX = axisW;
      const plotW = w - axisW;
      if (plotW <= 0) return;

      if (module.id === 'gpu') {
        ctx.font = '9px Menlo, Consolas, monospace';
        ctx.fillStyle = LABEL;
        for (const ref of [16.7, 33.3]) {
          const y = padTop + plotH - (ref / maxY) * plotH;
          if (y < padTop || y > padTop + plotH) continue;
          ctx.strokeStyle = GRID;
          ctx.beginPath();
          ctx.moveTo(plotX, y);
          ctx.lineTo(w, y);
          ctx.stroke();
          ctx.fillText(formatChartAxisValue(ref), 4, Math.max(10, y - 2));
        }
      }

      // 所有模块都保留同宽的纵轴与整数标签，帧柱/折线从同一 X 坐标开始。
      ctx.strokeStyle = GRID;
      ctx.beginPath();
      ctx.moveTo(plotX, padTop + plotH);
      ctx.lineTo(w, padTop + plotH);
      ctx.stroke();
      ctx.font = '9px Menlo, Consolas, monospace';
      ctx.fillStyle = LABEL;
      ctx.fillText(formatChartAxisValue(maxY), 4, 11);
      ctx.fillText('0', 4, padTop + plotH - 2);
      ctx.strokeStyle = GRID;
      ctx.beginPath();
      ctx.moveTo(plotX - 0.5, padTop);
      ctx.lineTo(plotX - 0.5, padTop + plotH);
      ctx.stroke();

      if (frames.length === 0) return;

      const barW = plotW / frames.length;
      ctx.save();
      ctx.beginPath();
      ctx.rect(plotX, padTop, plotW, plotH);
      ctx.clip();
      if (isCpu) {
        drawCpuAreas(ctx, plottedSeries, frames.map((frame) => frame.frameIndex), {
          left: plotX, bottom: padTop + plotH, width: plotW, height: plotH, max: maxY,
        });
      } else if (isMemory) {
        // Memory 模块与 CPU 的堆叠区不同：Editor 将异单位的计数器画成独立折线。
        for (const s of series) {
          const metric = memoryMetricForSeries(s.id);
          ctx.fillStyle = s.color;
          ctx.strokeStyle = s.color;
          ctx.lineWidth = 1;
          ctx.beginPath();
          let drawing = false;
          for (let i = 0; i < frames.length; i++) {
            if (metric && !memoryMetricAvailable(frames[i], metric)) {
              drawing = false;
              continue;
            }
            const y = padTop + plotH - ((s.values[i] || 0) / maxY) * plotH;
            if (frames.length === 1) {
              ctx.fillRect(plotX + plotW / 2 - 1.5, y - 1.5, 3, 3);
              continue;
            }
            const x = plotX + (i / (frames.length - 1)) * plotW;
            if (!drawing) {
              ctx.moveTo(x, y);
              drawing = true;
            } else {
              ctx.lineTo(x, y);
            }
          }
          ctx.stroke();
        }
      } else if (module.stacked) {
        for (let i = 0; i < frames.length; i++) {
          let acc = 0;
          for (const s of series) {
            const v = Math.max(0, s.values[i] || 0);
            const bh = (v / maxY) * plotH;
            ctx.fillStyle = s.color;
            ctx.fillRect(plotX + i * barW, padTop + plotH - acc - bh, Math.max(barW, 0.5), bh);
            acc += bh;
          }
        }
      } else {
        const inner = Math.max(barW / Math.max(series.length, 1), 0.4);
        for (let i = 0; i < frames.length; i++) {
          series.forEach((s, si) => {
            const v = Math.max(0, s.values[i] || 0);
            const bh = (v / maxY) * plotH;
            ctx.fillStyle = s.color;
            ctx.globalAlpha = 0.9;
            ctx.fillRect(plotX + i * barW + si * inner, padTop + plotH - bh, Math.max(inner - 0.3, 0.4), bh);
          });
        }
        ctx.globalAlpha = 1;
      }

      ctx.restore();

      if (isCpu) {
        ctx.font = '9px Menlo, Consolas, monospace';
        for (const fps of [60, 30]) {
          const y = padTop + plotH - (1000 / fps / maxY) * plotH;
          if (y < padTop || y > padTop + plotH) continue;
          ctx.strokeStyle = GRID;
          ctx.beginPath();
          ctx.moveTo(plotX, y);
          ctx.lineTo(w, y);
          ctx.stroke();
          const label = `${fps} FPS`;
          const textY = Math.max(13, y - 2);
          ctx.fillStyle = 'rgba(14, 17, 23, 0.85)';
          ctx.fillRect(plotX + 3, textY - 8, ctx.measureText(label).width + 4, 10);
          ctx.fillStyle = SELECT;
          ctx.fillText(label, plotX + 5, textY);
        }
      }

      // 当帧值超过手动上限时，柱体保持等比裁切，并以高对比红色顶端帽线标识。
      if (supportsOverflowIndicator) {
        for (let i = 0; i < frames.length; i++) {
          const value = module.stacked
            ? plottedSeries.reduce((sum, s) => sum + Math.max(0, s.values[i] || 0), 0)
            : plottedSeries.reduce((peak, s) => Math.max(peak, s.values[i] || 0), 0);
          if (value <= maxY) continue;
          const x = plotX + i * barW;
          const markWidth = Math.max(barW, 1);
          const markHeight = Math.min(5, plotH);
          ctx.fillStyle = OVERFLOW;
          ctx.fillRect(x, padTop, markWidth, markHeight);
          // 在宽度足够时加白色尖角，避免与任何分类色块混淆。
          if (barW >= 4) {
            const center = x + barW / 2;
            const size = Math.min(3, barW / 3);
            ctx.strokeStyle = '#fff';
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.moveTo(center - size, padTop + markHeight - 1);
            ctx.lineTo(center, padTop + 1);
            ctx.lineTo(center + size, padTop + markHeight - 1);
            ctx.stroke();
          }
        }
      }

      if (rangeStart !== undefined && rangeEnd !== undefined && rangeEnd >= 0 && rangeStart < frames.length) {
        const left = plotX + Math.max(0, rangeStart) * barW;
        const right = plotX + Math.min(frames.length, rangeEnd + 1) * barW;
        ctx.fillStyle = 'rgba(56, 189, 248, 0.2)';
        ctx.fillRect(left, 0, right - left, h);
        ctx.strokeStyle = 'rgba(56, 189, 248, 0.85)';
        ctx.strokeRect(left + 0.5, 0.5, Math.max(0, right - left - 1), h - 1);
      }
      if (selectedIndex >= 0 && selectedIndex < frames.length) {
        const x = plotX + selectedIndex * barW + barW / 2;
        ctx.strokeStyle = SELECT;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, h);
        ctx.stroke();
      }
    };

    draw();
    // 直接观察 Canvas、其轨道容器并监听窗口尺寸：窗口最大化和兄弟轨道显隐
    // 均能使已写入 CSS 宽度的 canvas 重新按实际可用空间测量。
    const ro = new ResizeObserver(draw);
    ro.observe(wrap);
    if (wrap.parentElement) ro.observe(wrap.parentElement);
    window.addEventListener('resize', draw);
    return () => {
      ro.disconnect();
      window.removeEventListener('resize', draw);
    };
  }, [chartHeight, fillAvailableHeight, frames, isCpu, isMemory, layoutRevision, maxY, module, plottedSeries, rangeEnd, rangeStart, selectedIndex, series, supportsOverflowIndicator]);

  const pick = (clientX: number) => {
    if (!wrapRef.current || frames.length === 0) return null;
    const rect = wrapRef.current.getBoundingClientRect();
    return indexFromClick(clientX, rect.left + Y_AXIS_WIDTH, Math.max(1, rect.width - Y_AXIS_WIDTH), frames.length);
  };

  const adjustYAxis = (delta: number) => {
    if (!supportsAdjustableYAxis || !onYAxisMaxChange) return;
    const next = adjustCpuChartMax(yAxisMaxRef.current, delta);
    yAxisMaxRef.current = next;
    onYAxisMaxChange(next);
  };

  const resetYAxis = () => {
    yAxisMaxRef.current = autoMaxY;
    onYAxisMaxChange?.(isCpu ? undefined : DEFAULT_CPU_CHART_MAX_Y);
  };

  return (
    <div
      className={`profiler-chart ${isCpu ? 'profiler-chart--cpu' : ''} ${isMemory ? 'profiler-chart--memory' : ''} ${active ? 'is-active' : ''} ${fillAvailableHeight ? 'profiler-chart--fill' : ''}`}
      data-testid={`profiler-chart-${module.id}`}
      data-active={active ? 'true' : 'false'}
      data-memory={isMemory ? 'true' : undefined}
      data-y-max={isCpu ? maxY.toFixed(2) : undefined}
      data-y-axis-width={Y_AXIS_WIDTH}
      data-overflow-count={supportsOverflowIndicator ? overflowingFrames : undefined}
      onClick={onActivate}
    >
      <div className="profiler-chart-meta">
        <div className="profiler-chart-title-row">
          <div className="profiler-chart-title">{module.label}</div>
        </div>
        <div className="profiler-chart-headline mono">
          {headline ? formatProfilerValue(headline.value, headline.unit) : '--'}
        </div>
        <ul className="profiler-legend">
          {legendSeries.map((s) => {
            const value = s.values[selectedIndex];
            const metric = memoryMetricForSeries(s.id);
            const available = !isMemory || !selectedFrame || !metric || memoryMetricAvailable(selectedFrame, metric);
            return (
              <li key={s.id} className={available ? '' : 'is-unavailable'}>
                {isCpu ? (
                  <button
                    type="button"
                    className="profiler-legend-toggle"
                    aria-pressed={!hiddenSeries.includes(s.id)}
                    draggable
                    title="点击切换显示，拖动调整堆叠顺序"
                    onClick={(event) => {
                      event.stopPropagation();
                      setHiddenSeries((current) => current.includes(s.id)
                        ? current.filter((id) => id !== s.id) : [...current, s.id]);
                    }}
                    onDragStart={(event) => {
                      draggedSeries.current = s.id;
                      event.dataTransfer.setData('text/plain', s.id);
                      event.dataTransfer.effectAllowed = 'move';
                    }}
                    onDragOver={(event) => {
                      if (draggedSeries.current !== null) event.preventDefault();
                    }}
                    onDrop={(event) => {
                      const source = draggedSeries.current;
                      if (source === null) return;
                      event.preventDefault();
                      draggedSeries.current = null;
                      setSeriesOrder((current) => moveChartSeries(current, source, s.id));
                    }}
                    onDragEnd={() => { draggedSeries.current = null; }}
                  >
                    <span className="swatch" style={{ background: s.color }} />
                    <span className="profiler-legend-label">{s.label}</span>
                  </button>
                ) : (
                  <>
                    <span className="swatch" style={{ background: s.color }} />
                    <span className="profiler-legend-label">{s.label}</span>
                  </>
                )}
                <span className="mono profiler-legend-val">
                  {available && value != null ? formatProfilerValue(value, s.unit) : '—'}
                </span>
              </li>
            );
          })}
        </ul>
      </div>
      <div
        ref={wrapRef}
        className="profiler-chart-canvas"
        data-frame-range={rangeStart !== undefined && rangeEnd !== undefined ? `${rangeStart}:${rangeEnd}` : undefined}
        title={isCpu ? '滚轮上下滚动 · Ctrl+滚轮缩放时间轴 · Shift+拖动框选 Left Heavy 帧范围' : 'Ctrl+滚轮缩放，Shift+滚轮平移'}

        onLostPointerCapture={(e) => {
          if (middleDrag.current?.id === e.pointerId) {
            middleDrag.current = null;
            e.currentTarget.style.cursor = '';
          }
        }}
        onPointerDown={(e) => {
          if (e.button === 1 && pan) {
            e.preventDefault();
            const width = e.currentTarget.getBoundingClientRect().width;
            if (!width || !frames.length) return;
            middleDrag.current = { id: e.pointerId, x: e.clientX, start: pan.start, framesPerPixel: pan.visible / width };
            e.currentTarget.setPointerCapture?.(e.pointerId);
            e.currentTarget.style.cursor = 'grabbing';
            return;
          }
          if (e.button !== 0) return;
          const index = pick(e.clientX);
          if (index === null) return;
          e.stopPropagation();
          onActivate();
          e.currentTarget.setPointerCapture?.(e.pointerId);
          if (e.shiftKey && onSelectRange) {
            setRangeDraft({ anchor: index, end: index });
          } else {
            dragging.current = true;
            onSelectFrame(index);
          }
        }}
        onPointerMove={(e) => {
          const drag = middleDrag.current;
          if (drag && drag.id === e.pointerId) {
            const delta = Math.round((drag.x - e.clientX) * drag.framesPerPixel);
            pan?.move(drag.start + delta);
            return;
          }
          const index = pick(e.clientX);
          if (index === null) return;
          if (rangeDraft) setRangeDraft({ ...rangeDraft, end: index });
          else if (dragging.current) onSelectFrame(index);
        }}
        onPointerUp={(e) => {
          if (middleDrag.current?.id === e.pointerId) {
            middleDrag.current = null;
            e.currentTarget.style.cursor = '';
            return;
          }
          dragging.current = false;
          if (!rangeDraft) return;
          const end = pick(e.clientX) ?? rangeDraft.end;
          onSelectRange?.({ start: Math.min(rangeDraft.anchor, end), end: Math.max(rangeDraft.anchor, end) });
          setRangeDraft(null);
        }}
        onPointerCancel={(e) => {
          middleDrag.current = null;
          e.currentTarget.style.cursor = '';
          dragging.current = false;
          setRangeDraft(null);
        }}
      >
        <canvas ref={canvasRef} />
        {supportsAdjustableYAxis && (
          <div
            className="profiler-y-axis-control"
            role="slider"
            tabIndex={0}
            aria-label={`调整 ${module.label} 纵轴上限`}
            aria-valuemin={0.01}
            aria-valuemax={60000}
            aria-valuenow={Math.round(maxY * 100) / 100}
            aria-valuetext={`${maxY.toFixed(2)}ms`}
            title={isCpu ? '拖动调整上限，双击恢复自动比例' : '拖动或滚动调整上限，双击恢复 100ms'}
            onWheel={isCpu ? undefined : (e) => {
              e.preventDefault();
              e.stopPropagation();
              adjustYAxis(e.deltaY);
            }}
            onDoubleClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              resetYAxis();
            }}
            onPointerDown={(e) => {
              if (e.button !== 0) return;
              e.preventDefault();
              e.stopPropagation();
              axisDrag.current = { lastY: e.clientY };
              e.currentTarget.setPointerCapture(e.pointerId);
            }}
            onPointerMove={(e) => {
              if (!axisDrag.current) return;
              const delta = e.clientY - axisDrag.current.lastY;
              axisDrag.current.lastY = e.clientY;
              adjustYAxis(delta);
            }}
            onPointerUp={(e) => {
              axisDrag.current = null;
              try { e.currentTarget.releasePointerCapture(e.pointerId); } catch { /* capture may already be released */ }
            }}
            onPointerCancel={(e) => { axisDrag.current = null; }}
            onKeyDown={(e) => {
              if (e.key === 'ArrowUp') {
                e.preventDefault();
                adjustYAxis(-20);
              } else if (e.key === 'ArrowDown') {
                e.preventDefault();
                adjustYAxis(20);
              } else if (e.key === 'Home') {
                e.preventDefault();
                resetYAxis();
              }
            }}
          />
        )}
      </div>
    </div>
  );
}
