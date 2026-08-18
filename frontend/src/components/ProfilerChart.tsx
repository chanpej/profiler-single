import { useEffect, useMemo, useRef } from 'react';
import type { FrameData } from '../types';
import { chartMax, extractModuleSeries, type ModuleDef } from '../lib/profiler';
import { formatProfilerValue } from '../lib/format';

interface Props {
  module: ModuleDef;
  frames: FrameData[];
  selectedIndex: number;
  active: boolean;
  height?: number;
  onSelectFrame: (index: number) => void;
  onActivate: () => void;
  onWheel?: (e: React.WheelEvent<HTMLElement>) => void;
}

const GRID = 'rgba(148, 163, 184, 0.14)';
const SELECT = 'rgba(226, 232, 240, 0.95)';
const LABEL = 'rgba(148, 163, 184, 0.75)';

function indexFromClick(clientX: number, wrap: HTMLElement, count: number): number {
  if (count <= 0) return 0;
  const rect = wrap.getBoundingClientRect();
  const x = Math.min(Math.max(clientX - rect.left, 0), rect.width);
  return Math.min(count - 1, Math.floor((x / rect.width) * count));
}

/**
 * Unity Profiler 风格模块图：左侧图例（含当前帧数值）+ 右侧 Canvas 堆叠/叠加柱。
 * 点击或拖拽图表选帧，点击整行激活该模块的详情面板。
 */
export function ProfilerChart({
  module, frames, selectedIndex, active, height = 88, onSelectFrame, onActivate, onWheel,
}: Props) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const dragging = useRef(false);
  const series = useMemo(() => extractModuleSeries(module, frames), [module, frames]);
  const maxY = useMemo(() => chartMax(module, series), [module, series]);
  const headline = frames[selectedIndex] ? module.headline(frames[selectedIndex]) : null;

  useEffect(() => {
    const canvas = canvasRef.current;
    const wrap = wrapRef.current;
    if (!canvas || !wrap) return;

    const draw = () => {
      const dpr = window.devicePixelRatio || 1;
      const w = wrap.clientWidth;
      const h = height;
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

      if (module.id === 'cpu' || module.id === 'gpu') {
        ctx.font = '9px Menlo, Consolas, monospace';
        ctx.fillStyle = LABEL;
        for (const ref of [16.7, 33.3]) {
          const y = padTop + plotH - (ref / maxY) * plotH;
          ctx.strokeStyle = GRID;
          ctx.beginPath();
          ctx.moveTo(0, y);
          ctx.lineTo(w, y);
          ctx.stroke();
          ctx.fillText(`${ref.toFixed(2)}ms`, 4, Math.max(10, y - 2));
        }
      }

      if (frames.length === 0) return;

      const barW = w / frames.length;
      if (module.stacked) {
        for (let i = 0; i < frames.length; i++) {
          let acc = 0;
          for (const s of series) {
            const v = s.values[i] || 0;
            const bh = (v / maxY) * plotH;
            ctx.fillStyle = s.color;
            ctx.fillRect(i * barW, padTop + plotH - acc - bh, Math.max(barW, 0.5), bh);
            acc += bh;
          }
        }
      } else {
        const inner = Math.max(barW / Math.max(series.length, 1), 0.4);
        for (let i = 0; i < frames.length; i++) {
          series.forEach((s, si) => {
            const v = s.values[i] || 0;
            const bh = (v / maxY) * plotH;
            ctx.fillStyle = s.color;
            ctx.globalAlpha = 0.9;
            ctx.fillRect(i * barW + si * inner, padTop + plotH - bh, Math.max(inner - 0.3, 0.4), bh);
          });
        }
        ctx.globalAlpha = 1;
      }

      if (selectedIndex >= 0 && selectedIndex < frames.length) {
        const x = selectedIndex * barW + barW / 2;
        ctx.strokeStyle = SELECT;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, h);
        ctx.stroke();
      }
    };

    draw();
    const ro = new ResizeObserver(draw);
    ro.observe(wrap);
    return () => ro.disconnect();
  }, [frames, height, maxY, module, selectedIndex, series]);

  const pick = (clientX: number) => {
    if (!wrapRef.current || frames.length === 0) return;
    onSelectFrame(indexFromClick(clientX, wrapRef.current, frames.length));
  };

  return (
    <div
      className={`profiler-chart ${active ? 'is-active' : ''}`}
      data-testid={`profiler-chart-${module.id}`}
      data-active={active ? 'true' : 'false'}
      onClick={onActivate}
    >
      <div className="profiler-chart-meta">
        <div className="profiler-chart-title">{module.label}</div>
        <div className="profiler-chart-headline mono">
          {headline ? formatProfilerValue(headline.value, headline.unit) : '--'}
        </div>
        <ul className="profiler-legend">
          {series.map((s) => {
            const value = s.values[selectedIndex];
            return (
              <li key={s.id}>
                <span className="swatch" style={{ background: s.color }} />
                <span className="profiler-legend-label">{s.label}</span>
                <span className="mono profiler-legend-val">
                  {value == null ? '' : formatProfilerValue(value, s.unit)}
                </span>
              </li>
            );
          })}
        </ul>
      </div>
      <div
        ref={wrapRef}
        className="profiler-chart-canvas"
        title="滚轮缩放，Shift+滚轮平移"
        onWheel={onWheel}
        onPointerDown={(e) => {
          e.stopPropagation();
          onActivate();
          dragging.current = true;
          (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
          pick(e.clientX);
        }}
        onPointerMove={(e) => {
          if (!dragging.current) return;
          pick(e.clientX);
        }}
        onPointerUp={() => {
          dragging.current = false;
        }}
        onPointerCancel={() => {
          dragging.current = false;
        }}
      >
        <canvas ref={canvasRef} />
      </div>
    </div>
  );
}
