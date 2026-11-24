import { useEffect, useMemo, useRef, useState } from 'react';
import type { FrameRange, HeavyProfile, HeavySample } from '../types';
import { formatMs } from '../lib/format';
import { categoryColor } from '../lib/hierarchy';
import { wheelZoomFactor } from '../lib/cpuTimeline';

interface Props {
  range: FrameRange;
  totalFrames: number;
  load: (start: number, end: number) => Promise<HeavyProfile>;
  onSelectRange: (range: FrameRange) => void;
}

type Metric = 'totalMs' | 'calls' | 'average' | 'selfMs';
const average = (node: HeavySample) => node.calls > 0 ? node.totalMs / node.calls : 0;
const rowHeight = 19;
const pageSize = 100;

interface Bar {
  node: HeavySample;
  start: number;
  end: number;
  depth: number;
}

interface FlameViewport {
  start: number;
  span: number;
  depth: number;
}

interface FlameDrag {
  pointerId: number;
  x: number;
  y: number;
  scrollTop: number;
  mode: 'select' | 'pan' | 'zoom';
  viewport: FlameViewport;
  moved: boolean;
}

function zoomFlame(current: FlameViewport, total: number, factor: number, ratio: number): FlameViewport | null {
  const minimum = Math.min(current.span, total / 1_000_000);
  const span = Math.min(total, Math.max(minimum, current.span * factor));
  if (span === total) return null;
  return {
    start: Math.max(0, Math.min(current.start + (current.span - span) * ratio, total - span)),
    span,
    depth: factor > 1 ? 0 : current.depth,
  };
}

function flameStats(node: HeavySample) {
  return <span className="mono">{formatMs(node.totalMs)} · {node.calls} 次 · 平均 {formatMs(average(node))}</span>;
}

function FlameGraph({ roots }: { roots: HeavySample[] }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const scroller = useRef<HTMLDivElement>(null);
  const drag = useRef<FlameDrag | null>(null);
  const [pointer, setPointer] = useState<{ x: number; y: number } | null>(null);
  const [selected, setSelected] = useState<Bar | null>(null);
  const [panning, setPanning] = useState(false);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewport, setViewport] = useState<FlameViewport | null>(null);
  const layout = useMemo(() => {
    const rows: Bar[][] = [];
    const add = (nodes: HeavySample[], start: number, depth: number, limit: number) => {
      for (const node of nodes) {
        const end = Math.min(limit, start + Math.max(0, node.totalMs));
        if (end > start) {
          (rows[depth] ??= []).push({ node, start, end, depth });
          if (node.children) add(node.children, start, depth + 1, end);
        }
        start = end;
      }
    };
    const span = roots.reduce((total, root) => total + Math.max(0, root.totalMs), 0);
    add(roots, 0, 0, span);
    return { rows, span };
  }, [roots]);
  const first = viewport?.start ?? 0;
  const span = viewport?.span ?? layout.span;
  const top = viewport?.depth ?? 0;
  const height = Math.max(rowHeight, (layout.rows.length - top) * rowHeight);

  useEffect(() => {
    const element = scroller.current;
    if (!element) return;
    const onWheel = (event: WheelEvent) => {
      const bounds = canvas.current?.getBoundingClientRect();
      if (!bounds || bounds.width <= 0 || layout.span <= 0 || (!event.deltaX && !event.deltaY)) return;
      event.preventDefault();
      const ratio = Math.min(1, Math.max(0, (event.clientX - bounds.left) / bounds.width));
      setPointer({ x: event.clientX, y: event.clientY });
      setViewport((current) => {
        if (event.shiftKey || event.deltaX) {
          if (!current) return null;
          const shift = (event.deltaX || event.deltaY) * current.span / bounds.width;
          return { ...current, start: Math.max(0, Math.min(current.start + shift, layout.span - current.span)) };
        }
        return zoomFlame(current ?? { start: 0, span: layout.span, depth: 0 }, layout.span, wheelZoomFactor(event.deltaY), ratio);
      });
    };
    element.addEventListener('wheel', onWheel, { passive: false });
    return () => element.removeEventListener('wheel', onWheel);
  }, [layout.span]);

  useEffect(() => {
    const element = canvas.current;
    if (!element) return;
    const draw = () => {
      const width = element.clientWidth;
      if (width <= 0 || span <= 0) return;
      const ratio = window.devicePixelRatio || 1;
      element.width = Math.round(width * ratio);
      element.height = Math.round(height * ratio);
      const ctx = element.getContext('2d');
      if (!ctx) return;
      ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
      ctx.font = '11px Consolas, monospace';
      ctx.textBaseline = 'middle';
      for (let depth = top; depth < layout.rows.length; depth++) {
        const y = (depth - top) * rowHeight;
        for (const bar of layout.rows[depth]) {
          if (bar.end <= first || bar.start >= first + span) continue;
          const left = Math.max(0, (bar.start - first) / span * width);
          const right = Math.min(width, (bar.end - first) / span * width);
          const barWidth = right - left;
          if (barWidth < 0.5) continue;
          ctx.fillStyle = categoryColor(bar.node.category, bar.node.name);
          ctx.fillRect(left, y, Math.max(0.5, barWidth - 1), rowHeight - 1);
          if (bar === selected) {
            ctx.strokeStyle = 'rgba(248, 250, 252, 0.95)';
            ctx.lineWidth = 1.5;
            ctx.strokeRect(left + 0.5, y + 0.5, Math.max(0, barWidth - 1), rowHeight - 2);
          }
          if (barWidth >= 22) {
            ctx.save();
            ctx.beginPath();
            ctx.rect(left + 3, y, barWidth - 6, rowHeight);
            ctx.clip();
            ctx.fillStyle = '#fff';
            ctx.shadowColor = '#101522';
            ctx.shadowBlur = 3;
            ctx.fillText(bar.node.name, left + 4, y + rowHeight / 2);
            ctx.restore();
          }
        }
      }
    };
    draw();
    const observer = new ResizeObserver(draw);
    observer.observe(element);
    return () => observer.disconnect();
  }, [first, height, layout, selected, span, top]);

  const pick = (clientX: number, clientY: number) => {
    const rect = canvas.current?.getBoundingClientRect();
    if (!rect || rect.width <= 0 || clientX < rect.left || clientX >= rect.left + rect.width || clientY < rect.top) return null;
    const time = first + (clientX - rect.left) / rect.width * span;
    const depth = top + Math.floor((clientY - rect.top) / rowHeight);
    return layout.rows[depth]?.find((bar) => time >= bar.start && time < bar.end) ?? null;
  };

  const pickHover = pointer && pick(pointer.x, pointer.y);
  const plotBounds = scroller.current?.getBoundingClientRect();
  const canvasBounds = canvas.current?.getBoundingClientRect();
  const tipWidth = Math.min(320, scroller.current?.clientWidth || plotBounds?.width || 0);
  const anchorTip = (x: number, below: number, above: number) => ({
    left: Math.max(0, Math.min(x, (scroller.current?.clientWidth || plotBounds?.width || 0) - tipWidth)),
    top: below + 28 <= (scroller.current?.clientHeight || plotBounds?.height || 0) || above < 0 ? below : above,
    maxWidth: tipWidth,
  });
  const hoverTip = pickHover && pickHover !== selected && pointer && plotBounds
    ? anchorTip(pointer.x - plotBounds.left + 12, pointer.y - plotBounds.top + 12, pointer.y - plotBounds.top - 34) : null;
  const selectedRow = selected ? (selected.depth - top) * rowHeight - scrollTop : -rowHeight;
  const pinnedTip = selected && canvasBounds && plotBounds && selected.depth >= top && selected.end > first && selected.start < first + span &&
    selectedRow + rowHeight > 0 && selectedRow < (scroller.current?.clientHeight || plotBounds.height)
    ? anchorTip(Math.max(0, (selected.start - first) / span * canvasBounds.width), selectedRow + rowHeight + 4, selectedRow - 28) : null;

  const reset = () => { setViewport(null); setPointer(null); };
  const focus = (bar: Bar) => {
    setViewport({ start: bar.start, span: bar.end - bar.start, depth: bar.depth });
    setPointer(null);
    if (scroller.current) scroller.current.scrollTop = 0;
    setScrollTop(0);
  };

  if (!layout.span) return <p className="dim">当前线程的采样没有可绘制的耗时。</p>;
  return <>
    <div className="heavy-flame-meta">
      <span data-testid="heavy-flame-stats">{selected
        ? <><strong>{selected.node.name}</strong> · {flameStats(selected.node)} · 自身 {formatMs(selected.node.selfMs)}</>
        : '左键选中 · 中键拖动 · 滚轮缩放 · 双击聚焦'}</span>
      {viewport && <button type="button" className="btn ghost" onClick={reset}>恢复全图</button>}
    </div>
    <div className="heavy-flame-plot">
      <div ref={scroller} className={panning ? 'heavy-flame-scroll is-panning' : 'heavy-flame-scroll'} tabIndex={0}
        onKeyDown={(event) => {
          const key = event.key.toLowerCase();
          if (key !== 'a' && key !== 'f') return;
          event.preventDefault();
          event.stopPropagation();
          if (key === 'f' && selected) focus(selected);
          else reset();
        }}
        onContextMenu={(event) => { if (event.altKey) event.preventDefault(); }}
        onPointerDown={(event) => {
          if (drag.current || (event.button !== 0 && event.button !== 1 && !(event.altKey && event.button === 2))) return;
          if (event.button === 1) {
            event.preventDefault();
            setPanning(true);
            setPointer(null);
          }
          event.currentTarget.focus({ preventScroll: true });
          event.currentTarget.setPointerCapture?.(event.pointerId);
          drag.current = {
            pointerId: event.pointerId, x: event.clientX, y: event.clientY, scrollTop: event.currentTarget.scrollTop,
            mode: event.button === 1 ? 'pan' : event.button === 2 ? 'zoom' : 'select',
            viewport: { start: first, span, depth: top }, moved: false,
          };
        }}
        onPointerMove={(event) => {
          const active = drag.current;
          if (!active) { setPointer({ x: event.clientX, y: event.clientY }); return; }
          if (active.pointerId !== event.pointerId) return;
          const dx = event.clientX - active.x;
          const dy = event.clientY - active.y;
          active.moved ||= Math.abs(dx) > 3 || Math.abs(dy) > 3;
          if (!active.moved) return;
          setPointer(null);
          const bounds = canvas.current?.getBoundingClientRect();
          if (!bounds || bounds.width <= 0) return;
          if (active.mode === 'pan') {
            event.currentTarget.scrollTop = active.scrollTop - dy;
            setScrollTop(event.currentTarget.scrollTop);
            const start = active.viewport.start - dx * active.viewport.span / bounds.width;
            if (active.viewport.span < layout.span) {
              setViewport({ ...active.viewport, start: Math.max(0, Math.min(start, layout.span - active.viewport.span)) });
            }
          } else if (active.mode === 'zoom') {
            const ratio = Math.min(1, Math.max(0, (active.x - bounds.left) / bounds.width));
            setViewport(zoomFlame(active.viewport, layout.span, Math.exp(-dx / 160), ratio));
          }
        }}
        onPointerUp={(event) => {
          const active = drag.current;
          if (!active || active.pointerId !== event.pointerId) return;
          drag.current = null;
          setPanning(false);
          if (active.mode !== 'select' || active.moved || Math.abs(event.clientX - active.x) > 3 || Math.abs(event.clientY - active.y) > 3) return;
          setPointer({ x: event.clientX, y: event.clientY });
          setSelected(pick(event.clientX, event.clientY));
        }}
        onLostPointerCapture={(event) => {
          if (drag.current?.pointerId !== event.pointerId) return;
          drag.current = null;
          setPanning(false);
        }}
        onPointerCancel={() => { drag.current = null; setPanning(false); setPointer(null); }}
        onPointerLeave={() => setPointer(null)}
        onScroll={(event) => { setScrollTop(event.currentTarget.scrollTop); setPointer(null); }}
        onDoubleClick={(event) => {
          const bar = pick(event.clientX, event.clientY);
          if (!bar) return;
          setSelected(bar);
          focus(bar);
        }}>
        <canvas ref={canvas} style={{ height, cursor: panning ? 'grabbing' : 'default' }} aria-label="按累计耗时排序的调用栈火焰图" />
      </div>
      {pinnedTip && selected && <div className="cpu-timeline-tip heavy-flame-tip" data-testid="heavy-flame-pinned-tip" style={pinnedTip}>
        <strong>{selected.node.name}</strong>{flameStats(selected.node)}
      </div>}
      {hoverTip && pickHover && <div className="cpu-timeline-tip heavy-flame-tip" role="tooltip" style={hoverTip}>
        <strong>{pickHover.node.name}</strong>{flameStats(pickHover.node)}
      </div>}
    </div>
  </>;
}

export function CpuLeftHeavy({ range: { start, end }, totalFrames, load, onSelectRange }: Props) {
  const [profile, setProfile] = useState<HeavyProfile | null>(null);
  const [error, setError] = useState('');
  const [threadID, setThreadID] = useState('main');
  const [metric, setMetric] = useState<Metric>('totalMs');
  const [page, setPage] = useState(0);
  useEffect(() => {
    let active = true;
    setProfile(null);
    setError('');
    setPage(0);
    load(start, end).then((result) => {
      if (active) setProfile(result);
    }).catch((reason) => {
      if (active) setError(String(reason));
    });
    return () => { active = false; };
  }, [start, end, load]);

  const threads = profile?.threads ?? [];
  const thread = threads.find((candidate) => candidate.id === threadID) ?? threads[0];
  const functions = useMemo(() => {
    const samples = thread?.functions ?? [];
    if (metric === 'totalMs') return samples;
    const value = metric === 'average' ? average : (sample: HeavySample) => sample[metric];
    return [...samples].sort((first, second) => value(second) - value(first));
  }, [thread, metric]);
  const pages = Math.ceil(functions.length / pageSize);
  return <section className="cpu-left-heavy" aria-label="Left Heavy">
    <form key={`${start}:${end}`} className="heavy-controls" onSubmit={(event) => {
      event.preventDefault();
      const data = new FormData(event.currentTarget);
      const first = Number(data.get('first')) - 1;
      const last = Number(data.get('last')) - 1;
      onSelectRange({ start: Math.min(first, last), end: Math.max(first, last) });
    }}>
      <label>起始帧序号 <input name="first" type="number" min={1} max={totalFrames} defaultValue={start + 1} required /></label>
      <label>结束帧序号 <input name="last" type="number" min={1} max={totalFrames} defaultValue={end + 1} required /></label>
      <button className="btn ghost" type="submit">分析范围</button>
      <span className="dim">也可在 CPU 图上按住 Shift 拖选</span>
    </form>
    {error ? <p role="alert">{error}</p> : !profile ? <p role="status">正在汇总采样…</p> : <>
      <div className="heavy-controls">
        <span className="mono">#{profile.firstFrame}–#{profile.lastFrame} · {profile.frameCount} 帧</span>
        {thread && <label>线程 <select value={thread.id} onChange={(event) => { setThreadID(event.target.value); setPage(0); }}>
          {threads.map((candidate) => <option key={candidate.id} value={candidate.id}>{candidate.name}</option>)}
        </select></label>}
      </div>
      {thread ? <>
        <FlameGraph key={`${start}:${end}:${thread.id}`} roots={thread.roots} />
        <table className="profiler-table heavy-table" aria-label="函数汇总">
          <thead><tr><th>函数 / 采样</th>
            {([['calls', '调用次数'], ['average', '平均耗时'], ['totalMs', '累计耗时'], ['selfMs', '自身耗时']] as const).map(([id, label]) =>
              <th key={id} aria-sort={metric === id ? 'descending' : 'none'}>
                <button type="button" onClick={() => { setMetric(id); setPage(0); }}>{label}{metric === id ? ' ↓' : ''}</button>
              </th>)}
          </tr></thead>
          <tbody>{functions.slice(page * pageSize, (page + 1) * pageSize).map((sample) => <tr key={JSON.stringify([sample.name, sample.category])}>
            <td title={sample.category}>{sample.name}</td><td>{sample.calls}</td><td>{formatMs(average(sample))}</td>
            <td>{formatMs(sample.totalMs)}</td><td>{formatMs(sample.selfMs)}</td>
          </tr>)}</tbody>
        </table>
        {pages > 1 && <div className="heavy-controls">
          <button type="button" className="btn ghost" disabled={page === 0} onClick={() => setPage(page - 1)}>上一页</button>
          <span>{page + 1} / {pages}</span>
          <button type="button" className="btn ghost" disabled={page + 1 >= pages} onClick={() => setPage(page + 1)}>下一页</button>
        </div>}
      </> : <p className="dim">所选帧没有 CPU 调用采样。</p>}
    </>}
  </section>;
}
