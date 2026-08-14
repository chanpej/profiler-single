import { useEffect, useRef } from 'react';
import type { FrameData } from '../types';

interface Props {
  frames: FrameData[];
  height?: number;
}

const GOOD = '#34d399';
const WARN = '#fbbf24';
const BAD = '#f87171';
const GRID = 'rgba(148, 163, 184, 0.12)';
const LABEL = 'rgba(148, 163, 184, 0.7)';

function barColor(cpuMs: number): string {
  if (cpuMs <= 20) return GOOD;
  if (cpuMs <= 33.4) return WARN;
  return BAD;
}

/**
 * 帧耗时时间轴图表。直接用 Canvas 绘制，避免图表库的虚拟 DOM 开销，
 * 每次数据更新只做一次全量重绘（千级柱条在 1ms 内完成）。
 */
export function FrameChart({ frames, height = 220 }: Props) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const wrapRef = useRef<HTMLDivElement | null>(null);

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
      ctx.scale(dpr, dpr);
      ctx.clearRect(0, 0, w, h);

      const padTop = 8;
      const padBottom = 18;
      const plotH = h - padTop - padBottom;
      const maxMs = Math.max(50, ...frames.map((f) => f.cpuTimeMs)) * 1.1;

      // 网格与参考线（16.7ms / 33.3ms）。
      ctx.font = '10px Menlo, Consolas, monospace';
      ctx.fillStyle = LABEL;
      for (const ref of [16.7, 33.3]) {
        const y = padTop + plotH - (ref / maxMs) * plotH;
        ctx.strokeStyle = GRID;
        ctx.beginPath();
        ctx.moveTo(0, y);
        ctx.lineTo(w, y);
        ctx.stroke();
        ctx.fillText(`${ref}ms`, 4, y - 3);
      }

      if (frames.length === 0) {
        ctx.fillStyle = LABEL;
        ctx.font = '12px system-ui, sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText('暂无数据 — 请使用 USB 连接设备', w / 2, h / 2);
        ctx.textAlign = 'left';
        return;
      }

      // 帧耗时柱状条。
      const barW = w / frames.length;
      for (let i = 0; i < frames.length; i++) {
        const f = frames[i];
        const bh = Math.min(plotH, (f.cpuTimeMs / maxMs) * plotH);
        ctx.fillStyle = barColor(f.cpuTimeMs);
        ctx.globalAlpha = 0.85;
        ctx.fillRect(i * barW, padTop + plotH - bh, Math.max(barW - 0.5, 0.5), bh);
      }
      ctx.globalAlpha = 1;

      // 底部时间标签（首尾帧序号）。
      ctx.fillStyle = LABEL;
      ctx.font = '10px Menlo, Consolas, monospace';
      ctx.fillText(`#${frames[0].frameIndex}`, 4, h - 5);
      const lastLabel = `#${frames[frames.length - 1].frameIndex}`;
      ctx.fillText(lastLabel, w - ctx.measureText(lastLabel).width - 4, h - 5);
    };

    draw();
    const ro = new ResizeObserver(draw);
    ro.observe(wrap);
    return () => ro.disconnect();
  }, [frames, height]);

  return (
    <div ref={wrapRef} className="chart-wrap" data-testid="frame-chart">
      <canvas ref={canvasRef} />
    </div>
  );
}
