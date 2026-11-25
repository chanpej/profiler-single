import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ClipboardCopy } from 'lucide-react';
import type { CpuTimelineFrame, FrameData, FrameRange, HeavyProfile, ProfilerModuleId, SampleNode } from '../types';
import { memoryEditorSummary, memoryMetricAvailable, moduleBreakdown, moduleById, renderingCounters, frameTimeMs, gpuOrFrameMs } from '../lib/profiler';
import { formatCount, formatFps, formatGCAlloc, formatMB1, formatMs, formatPct, formatProfilerValue } from '../lib/format';
import { categoryColor, flattenVisible, mergeExpanded, mergeSampleNodes } from '../lib/hierarchy';
import { copyText } from '../lib/clipboard';
import { CpuTimeline } from './CpuTimeline';
import { CpuLeftHeavy } from './CpuLeftHeavy';
import {
  callstackRatioFromPointer,
  loadCallstackRatio,
  loadOverviewWidth,
  METRIC_HEADERS,
  metricColumnWidths,
  overviewWidthFromDrag,
  saveCallstackRatio,
  saveOverviewWidth,
} from '../lib/hierarchyLayout';

interface Props {
  moduleId: ProfilerModuleId;
  frame: FrameData | null;
  hierarchy?: SampleNode[];
  callStacks?: boolean;
  getTimeline?: (index: number, frameIndex: number) => Promise<CpuTimelineFrame[]>;
  timelineSelection?: number;
  timelineFallback?: { index: number; frame: FrameData } | null;
  onTimelineSeek?: (index: number) => void;
  onTimelineRange?: (range: FrameRange | null) => void;
  refreshTimeline?: boolean;
  getHeavyProfile?: (start: number, end: number) => Promise<HeavyProfile>;
  heavyRange?: FrameRange | null;
  onHeavyRange?: (range: FrameRange | null) => void;
  cpuTab?: CpuTab;
  onCpuTabChange?: (tab: CpuTab) => void;
  totalFrames?: number;
}

export type CpuTab = 'hierarchy' | 'timeline' | 'left-heavy';
type MemTab = 'simple' | 'detailed';

/** 选中帧的模块详情：还原 Unity Profiler 底部面板。 */
export function ProfilerDetail({ moduleId, frame, hierarchy = [], callStacks = false, getTimeline, timelineSelection = -1, timelineFallback, onTimelineSeek, refreshTimeline,
  getHeavyProfile, heavyRange, onHeavyRange, onTimelineRange, cpuTab: controlledCpuTab, onCpuTabChange, totalFrames = 0 }: Props) {
  const [localCpuTab, setLocalCpuTab] = useState<CpuTab>('hierarchy');
  const cpuTab = controlledCpuTab ?? localCpuTab;
  const setCpuTab = onCpuTabChange ?? setLocalCpuTab;
  const [memTab, setMemTab] = useState<MemTab>('simple');
  const mod = moduleById(moduleId);
  const timelineFrame = frame ?? timelineFallback?.frame;

  return (
    <div className="profiler-detail" data-testid="profiler-detail">
      <div className="profiler-detail-head">
        <h2>{mod.label}</h2>
        {moduleId === 'cpu' && (
          <div className="profiler-tabs" role="tablist" aria-label="CPU 视图">
            <button type="button" role="tab" aria-selected={cpuTab === 'hierarchy'} className={cpuTab === 'hierarchy' ? 'is-active' : ''} onClick={() => setCpuTab('hierarchy')}>Hierarchy</button>
            <button type="button" role="tab" aria-selected={cpuTab === 'timeline'} className={cpuTab === 'timeline' ? 'is-active' : ''} onClick={() => setCpuTab('timeline')}>Timeline</button>
            {getHeavyProfile && <button type="button" role="tab" aria-selected={cpuTab === 'left-heavy'} className={cpuTab === 'left-heavy' ? 'is-active' : ''} onClick={() => setCpuTab('left-heavy')}>Left Heavy</button>}
          </div>
        )}
        {moduleId === 'memory' && (
          <div className="profiler-tabs" role="tablist" aria-label="Memory 视图">
            <button type="button" role="tab" aria-selected={memTab === 'simple'} className={memTab === 'simple' ? 'is-active' : ''} onClick={() => setMemTab('simple')}>Simple</button>
            <button type="button" role="tab" aria-selected={memTab === 'detailed'} className={memTab === 'detailed' ? 'is-active' : ''} onClick={() => setMemTab('detailed')} title="Detailed 需要内存快照，独立采集暂不支持">Detailed</button>
          </div>
        )}
      </div>
      <div className="profiler-detail-body">
        {moduleId === 'cpu' && cpuTab === 'timeline' && timelineFrame ? (
          <TimelineDetail frame={timelineFrame} hierarchy={hierarchy} load={getTimeline} seek={onTimelineSeek}
            onVisibleRange={onTimelineRange}
            index={frame ? timelineSelection : (timelineFallback?.index ?? timelineSelection)} refresh={refreshTimeline} totalFrames={totalFrames} />
        ) : !frame ? (
          <p className="dim">选择一帧以查看详情</p>
        ) : moduleId === 'cpu' && cpuTab === 'left-heavy' && getHeavyProfile && onHeavyRange ? (
          <CpuLeftHeavy range={heavyRange ?? { start: timelineSelection, end: timelineSelection }}
            totalFrames={totalFrames} load={getHeavyProfile} onSelectRange={onHeavyRange} />
        ) : moduleId === 'cpu' && cpuTab === 'hierarchy' && hierarchy.length > 0 ? (
          <SampleHierarchy nodes={hierarchy} callStacks={callStacks} />
        ) : moduleId === 'memory' && memTab === 'detailed' ? (
          <p className="dim">Detailed 视图需要 Unity 内存快照，当前采集通道仅提供 Simple 计数器。</p>
        ) : moduleId === 'memory' ? (
          <MemorySimple frame={frame} />
        ) : moduleId === 'rendering' ? (
          (frame.drawCalls > 0 || frame.batches > 0)
            ? <CounterTable rows={renderingCounters(frame).map((r) => ({ label: r.label, value: formatCount(r.value) }))} />
            : <CounterTable rows={[
              { label: 'Frame Time', value: formatMs(frameTimeMs(frame)) },
              { label: 'FPS', value: formatFps(frame.fps) },
            ]} />
        ) : moduleId === 'audio' ? (
          <CounterTable rows={[
            { label: 'Playing Sources', value: String(frame.playingSources) },
            { label: 'Audio Voices', value: String(frame.audioVoices) },
            { label: 'Audio Memory', value: formatMB1(frame.audioUsedMB) },
          ]} />
        ) : moduleId === 'physics' ? (
          <CounterTable rows={[
            { label: 'Active Rigidbodies', value: String(frame.activeRigidbodies) },
            { label: 'Colliders', value: String(frame.colliders) },
            { label: 'Physics CPU', value: formatMs(frame.cpuPhysicsMs) },
          ]} />
        ) : moduleId === 'ui' ? (
          <CounterTable rows={[
            { label: 'UI Batches', value: String(frame.uiBatches) },
            { label: 'UI Vertices', value: formatCount(frame.uiVertices) },
            { label: 'UI CPU', value: formatMs(frame.cpuUiMs) },
          ]} />
        ) : moduleId === 'video' ? (
          <CounterTable rows={[
            { label: 'Video Memory', value: formatMB1(frame.videoUsedMB) },
          ]} />
        ) : moduleId === 'gpu' ? (
          <CounterTable rows={[
            { label: 'GPU Time', value: formatMs(gpuOrFrameMs(frame)) },
            { label: 'Frame Time', value: formatMs(frameTimeMs(frame)) },
            { label: 'Render Thread', value: formatMs(frame.renderMs) },
          ]} />
        ) : (
          <HierarchyTable rows={moduleBreakdown(mod, frame)} />
        )}
      </div>
    </div>
  );
}

function TimelineDetail({ frame, hierarchy, load, index, refresh, seek, totalFrames, onVisibleRange }: {
  frame: FrameData;
  hierarchy: SampleNode[];
  load?: (index: number, frameIndex: number) => Promise<CpuTimelineFrame[]>;
  index: number;
  refresh?: boolean;
  onVisibleRange?: (range: FrameRange | null) => void;
  totalFrames: number;
  seek?: (index: number) => void;
}) {
  const [loaded, setLoaded] = useState<{ index: number; frameIndex: number; rows: CpuTimelineFrame[] }>();
  const frameIndex = frame.frameIndex;
  const context = loaded?.index === index && loaded.frameIndex === frameIndex ? loaded.rows : undefined;
  const generation = useRef(0);
  const inFlight = useRef(new Set<string>());
  useEffect(() => {
    if (!load) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let remainingRefreshes = refresh ? 4 : 0;
    const fetchThreads = async () => {
      try {
        const rows = await load(index, frameIndex);
        if (active) setLoaded((previous) => {
          if (previous?.index !== index || previous.frameIndex !== frameIndex || !rows.length) {
            return { index, frameIndex, rows };
          }
          const first = rows[0].frameIndex;
          const last = rows[rows.length - 1].frameIndex;
          return { index, frameIndex, rows: [
            ...previous.rows.filter((row) => row.frameIndex < first), ...rows,
            ...previous.rows.filter((row) => row.frameIndex > last),
          ] };
        });
      } catch {
        if (active) setLoaded((previous) => previous?.index === index && previous.frameIndex === frameIndex ? previous : { index, frameIndex, rows: [] });
      } finally {
        if (active && remainingRefreshes > 0) {
          remainingRefreshes--;
          timer = setTimeout(() => { void fetchThreads(); }, 500);
        }
      }
    };
    void fetchThreads();
    return () => { active = false; generation.current++; inFlight.current.clear(); clearTimeout(timer); };
  }, [load, index, frameIndex, refresh]);
  const extend = useCallback(async (edge: CpuTimelineFrame, direction: -1 | 1) => {
    if (!load || !context) return;
    const center = context.findIndex((row) => row.selected);
    if (center < 0) return;
    const edgeIndex = index - center + (direction < 0 ? 0 : context.length - 1);
    const epoch = generation.current;
    const key = `${epoch}:${direction}:${edge.frameIndex}`;
    if (inFlight.current.has(key)) return;
    inFlight.current.add(key);
    try {
      const page = await load(edgeIndex, edge.frameIndex);
      if (epoch !== generation.current) return;
      const pivot = page.findIndex((row) => row.selected && row.frameIndex === edge.frameIndex);
      if (pivot < 0) return;
      const extension = (direction < 0 ? page.slice(0, pivot) : page.slice(pivot + 1)).map((row) => ({
        ...row, selected: false, offsetMs: edge.offsetMs + row.offsetMs - page[pivot].offsetMs,
      }));
      if (!extension.length) return;
      setLoaded((previous) => {
        if (previous?.index !== index || previous.frameIndex !== frameIndex) return previous;
        const rows = previous.rows;
        if (rows[direction < 0 ? 0 : rows.length - 1]?.frameIndex !== edge.frameIndex) return previous;
        return { ...previous, rows: direction < 0 ? [...extension, ...rows] : [...rows, ...extension] };
      });
    } catch {
      return;
    } finally {
      inFlight.current.delete(key);
    }
  }, [load, context, index, frameIndex]);
  const selectNeighbor = seek && context ? (neighbor: CpuTimelineFrame) => {
    const center = context.findIndex((candidate) => candidate.selected);
    const position = context.indexOf(neighbor);
    if (center >= 0 && position >= 0) seek(index + position - center);
  } : undefined;
  return <CpuTimeline frame={frame} hierarchy={load ? [] : hierarchy} context={context}
    selectionIndex={index} onVisibleRange={onVisibleRange}
    onExtend={extend}
    canPageLeft={!!context && index > context.findIndex((row) => row.selected)}
    canPageRight={!!context && index + context.length - context.findIndex((row) => row.selected) < totalFrames}
    loadingContext={!!load && context === undefined} onSelectNeighbor={selectNeighbor} />;
}

function SampleHierarchy({ nodes, callStacks }: { nodes: SampleNode[]; callStacks: boolean }) {
  const merged = useMemo(() => mergeSampleNodes(nodes), [nodes]);
  const touched = useRef(new Set<string>());
  const expandOpts = { revealCallstacks: callStacks };
  const [expanded, setExpanded] = useState(() => mergeExpanded(new Set(), merged, touched.current, expandOpts));
  const [selected, setSelected] = useState<string>('');
  const [overviewW, setOverviewW] = useState(loadOverviewWidth);
  const [callstackRatio, setCallstackRatio] = useState(loadCallstackRatio);
  const rootRef = useRef<HTMLDivElement>(null);
  const overviewDrag = useRef<{ startX: number; startW: number } | null>(null);
  const stackDrag = useRef(false);
  const overviewWRef = useRef(overviewW);
  overviewWRef.current = overviewW;
  const callstackRatioRef = useRef(callstackRatio);
  callstackRatioRef.current = callstackRatio;

  const visibleExpanded = useMemo(
    () => mergeExpanded(expanded, merged, touched.current, { revealCallstacks: callStacks }),
    [expanded, merged, callStacks],
  );
  const rows = useMemo(() => flattenVisible(merged, visibleExpanded), [merged, visibleExpanded]);
  // Unity Hierarchy 的 Total / Self 百分比以当前线程根采样的总耗时为基准。
  const rootTotalMs = useMemo(
    () => merged.reduce((max, node) => Math.max(max, node.totalMs), 0),
    [merged],
  );
  const metricValues = useMemo(
    () => rows.map((r) => [
      formatPct(rootTotalMs > 0 ? (r.node.totalMs / rootTotalMs) * 100 : 0),
      formatPct(rootTotalMs > 0 ? (r.node.selfMs / rootTotalMs) * 100 : 0),
      String(r.node.calls),
      formatGCAlloc(r.node.gcAllocKB),
      formatMs(r.node.totalMs),
      formatMs(r.node.selfMs),
    ]),
    [rows, rootTotalMs],
  );
  const metricWidths = useMemo(
    () => metricColumnWidths(METRIC_HEADERS, metricValues),
    [metricValues],
  );
  const selectedRow = rows.find((r) => r.path === selected);
  const stack = selectedRow?.node.callstack ?? [];
  const stackText = stack.join('\n');
  // 本帧是否真的带调用栈：Player 只在发生分配的帧写调用栈，空面板要能说清是哪种"空"。
  const frameHasStack = useMemo(() => rows.some((r) => (r.node.callstack?.length ?? 0) > 0), [rows]);

  const onOverviewPointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    overviewDrag.current = { startX: e.clientX, startW: overviewWRef.current };
    e.currentTarget.setPointerCapture?.(e.pointerId);
  }, []);

  const onOverviewPointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!overviewDrag.current) return;
    setOverviewW(overviewWidthFromDrag(overviewDrag.current.startW, overviewDrag.current.startX, e.clientX));
  }, []);

  const onOverviewPointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!overviewDrag.current) return;
    overviewDrag.current = null;
    try {
      e.currentTarget.releasePointerCapture?.(e.pointerId);
    } catch {
      // capture 可能已释放
    }
    saveOverviewWidth(overviewWRef.current);
  }, []);

  const onStackPointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    stackDrag.current = true;
    e.currentTarget.setPointerCapture?.(e.pointerId);
  }, []);

  const onStackPointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!stackDrag.current || !rootRef.current) return;
    const rect = rootRef.current.getBoundingClientRect();
    setCallstackRatio(callstackRatioFromPointer(rect.width, rect.left, e.clientX));
  }, []);

  const onStackPointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!stackDrag.current) return;
    stackDrag.current = false;
    try {
      e.currentTarget.releasePointerCapture?.(e.pointerId);
    } catch {
      // capture 可能已释放
    }
    saveCallstackRatio(callstackRatioRef.current);
  }, []);

  return (
    <div
      className="sample-hierarchy"
      data-testid="cpu-hierarchy"
      ref={rootRef}
      style={{
        '--overview-w': `${overviewW}px`,
        '--callstack-ratio': String(callstackRatio),
      } as React.CSSProperties}
    >
      <div className="hierarchy-pane">
        <div className="hierarchy-scroll" data-testid="hierarchy-scroll">
          <table className="profiler-table hierarchy-table" data-testid="hierarchy-table">
            <colgroup>
              <col className="col-overview" />
              {metricWidths.map((w, i) => (
                <col key={METRIC_HEADERS[i]} className="col-metric" style={{ width: w }} />
              ))}
            </colgroup>
            <thead>
              <tr>
                <th className="col-overview">
                  Overview
                  <div
                    className="col-resize"
                    role="separator"
                    aria-orientation="vertical"
                    aria-label="调整 Overview 列宽"
                    data-testid="overview-resize"
                    onPointerDown={onOverviewPointerDown}
                    onPointerMove={onOverviewPointerMove}
                    onPointerUp={onOverviewPointerUp}
                    onPointerCancel={onOverviewPointerUp}
                  />
                </th>
                {METRIC_HEADERS.map((header) => (
                  <th key={header} className="col-metric">{header}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((r, rowIndex) => (
                <tr
                  key={r.path}
                  className={selected === r.path ? 'is-selected' : ''}
                  onClick={() => setSelected(r.path)}
                >
                  <td className="col-overview">
                    <span className="hierarchy-cell" style={{ paddingLeft: r.depth * 14 }}>
                      {r.hasChildren ? (
                        <button
                          type="button"
                          className="hierarchy-toggle"
                          aria-label={visibleExpanded.has(r.path) ? '折叠' : '展开'}
                          onClick={(e) => {
                            e.stopPropagation();
                            touched.current.add(r.path);
                            setExpanded((cur) => {
                              const next = new Set(mergeExpanded(cur, merged, touched.current, { revealCallstacks: callStacks }));
                              if (next.has(r.path)) next.delete(r.path);
                              else next.add(r.path);
                              return next;
                            });
                          }}
                        >
                          {visibleExpanded.has(r.path) ? '▾' : '▸'}
                        </button>
                      ) : (
                        <span className="hierarchy-toggle is-leaf" />
                      )}
                      <span className="swatch" style={{ background: categoryColor(r.node.category, r.node.name) }} />
                      <span className="hierarchy-name" title={r.node.name}>{r.node.name}</span>
                    </span>
                  </td>
                  {metricValues[rowIndex].map((value, metricIndex) => (
                    <td key={METRIC_HEADERS[metricIndex]} className="mono col-metric">{value}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      {callStacks && (
        <>
          <div
            className="callstack-resize"
            role="separator"
            aria-orientation="vertical"
            aria-label="调整 Call Stacks 宽度"
            data-testid="callstack-resize"
            onPointerDown={onStackPointerDown}
            onPointerMove={onStackPointerMove}
            onPointerUp={onStackPointerUp}
            onPointerCancel={onStackPointerUp}
          />
          <div
            className="callstack-pane"
            data-testid="callstack-pane"
            style={{ width: `${callstackRatio * 100}%` }}
          >
            <div className="callstack-toolbar">
              <h3>Call Stacks</h3>
              <button
                type="button"
                className="btn ghost"
                aria-label="复制"
                title="复制"
                disabled={stack.length === 0}
                onClick={() => { void copyText(stackText); }}
              >
                <ClipboardCopy size={14} aria-hidden />
              </button>
            </div>
            <div className="callstack-body">
              {stack.length > 0 ? (
                <pre className="callstack-list">{stackText}</pre>
              ) : frameHasStack ? (
                <p className="dim">
                  本帧带调用栈的采样已自动展开（GC.Alloc 一类的分配采样），点中它即可查看。
                  IL2CPP 包里多数栈帧是引擎原生代码，没有符号可查，显示为十六进制地址。
                </p>
              ) : (
                <p className="dim">
                  本帧没有带调用栈的分配采样。调用栈只对开启 Call Stacks 之后采集的帧生效，
                  可在上方帧列表里挑一帧 GC Alloc 不为 0 的帧查看。
                </p>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}

function HierarchyTable({ rows }: { rows: ReturnType<typeof moduleBreakdown> }) {
  return (
    <table className="profiler-table">
      <thead>
        <tr>
          <th>Overview</th>
          <th>Time ms</th>
          <th>%</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.id}>
            <td>
              <span className="swatch" style={{ background: r.color }} />
              {r.label}
            </td>
            <td className="mono">{formatProfilerValue(r.value, r.unit)}</td>
            <td className="mono">{formatPct(r.pct)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function MemorySimple({ frame }: { frame: FrameData }) {
  const summary = memoryEditorSummary(frame);
  const value = (available: boolean, amount: number) => (available ? formatMB1(amount) : '—');
  const objectRows = [
    { label: 'Textures', count: frame.textureCount, size: frame.textureMemMB, available: memoryMetricAvailable(frame, 'textureCount') && memoryMetricAvailable(frame, 'texture') },
    { label: 'Meshes', count: frame.meshCount, size: frame.meshMemMB, available: memoryMetricAvailable(frame, 'meshCount') && memoryMetricAvailable(frame, 'mesh') },
    { label: 'Materials', count: frame.materialCount, size: frame.materialMemMB, available: memoryMetricAvailable(frame, 'materialCount') && memoryMetricAvailable(frame, 'material') },
    { label: 'Animation Clips', count: frame.animationClipCount, size: frame.animationClipMemMB, available: memoryMetricAvailable(frame, 'animationClipCount') && memoryMetricAvailable(frame, 'animationClip') },
    { label: 'Assets', count: frame.assetCount, available: memoryMetricAvailable(frame, 'assetCount') },
    { label: 'Game Objects', count: frame.gameObjectCount, available: memoryMetricAvailable(frame, 'gameObjectCount') },
    { label: 'Scene Objects', count: frame.sceneObjectCount, available: memoryMetricAvailable(frame, 'sceneObjectCount') },
  ];
  const trackedSegments = [
    { label: 'Tracked Memory', value: summary.trackedReserved, color: '#E8A838', available: summary.trackedAvailable },
    { label: 'Untracked Memory', value: summary.untracked, color: '#65717E', available: summary.untrackedAvailable, untracked: true },
  ];
  const breakdownSegments = [
    ...summary.breakdown.filter((row) => row.available).map((row) => ({ label: row.label, value: row.used, color: row.color })),
    ...(summary.untrackedAvailable ? [{ label: 'Untracked Memory', value: summary.untracked, color: '#65717E', untracked: true }] : []),
  ];

  return (
    <div className="memory-editor-simple" data-testid="memory-simple">
      <section className="memory-editor-section" aria-labelledby="committed-memory-title">
        <div className="memory-editor-heading">
          <h3 id="committed-memory-title">Total Committed Memory</h3>
          <span className="mono">{value(summary.committedAvailable, summary.committed)}</span>
        </div>
        <MemoryBar total={summary.committed} segments={trackedSegments} />
        <div className="memory-editor-rows">
          <MemoryValueRow label="Tracked Memory" available={summary.trackedAvailable} used={summary.trackedUsed} reserved={summary.trackedReserved} />
          <MemoryValueRow label="Untracked Memory" available={summary.untrackedAvailable} used={summary.untracked} />
        </div>
      </section>
      <section className="memory-editor-section" aria-labelledby="memory-gc-title">
        <div className="memory-editor-heading"><h3 id="memory-gc-title">GC Allocated In Frame</h3></div>
        <div className="memory-editor-rows">
          <MemoryValueRow
            label="GC Allocated In Frame"
            available={memoryMetricAvailable(frame, 'gcAllocationFrameCount') && memoryMetricAvailable(frame, 'gcAllocatedFrame')}
            used={frame.gcAllocatedFrameKB}
            count={frame.gcAllocationFrameCount}
          />
        </div>
      </section>
      <section className="memory-editor-section" aria-labelledby="memory-breakdown-title">
        <div className="memory-editor-heading">
          <h3 id="memory-breakdown-title">Total Memory Breakdown</h3>
          <span className="mono">{value(summary.committedAvailable, summary.committed)}</span>
        </div>
        <MemoryBar total={summary.committed} segments={breakdownSegments} />
        <div className="memory-editor-rows">
          {summary.breakdown.map((row) => (
            <MemoryValueRow key={row.id} label={row.label} color={row.color} available={row.available} used={row.used} reserved={row.reserved} />
          ))}
          <MemoryValueRow label="Untracked Memory" color="#65717E" available={summary.untrackedAvailable} used={summary.untracked} />
        </div>
      </section>
      <section className="memory-editor-section" aria-labelledby="object-statistics-title">
        <div className="memory-editor-heading"><h3 id="object-statistics-title">Object Statistics</h3></div>
        <table className="profiler-table memory-object-table">
          <thead><tr><th>Category</th><th>Count</th><th>Size</th></tr></thead>
          <tbody>
            {objectRows.map((row) => (
              <tr key={row.label} className={row.available ? '' : 'is-unavailable'}>
                <td>{row.label}</td>
                <td className="mono">{row.available ? formatCount(row.count) : '—'}</td>
                <td className="mono">{row.available && row.size != null ? formatMB1(row.size) : '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
      {!summary.committedAvailable && (
        <p className="dim memory-unavailable">
          当前帧未记录 App Committed Memory。已保存的 Memory 计数器（例如 Audio Used Memory）会正常显示；其余统计需在采集时由 Unity 写入，或使用原始 .uppc / .upac 存档。
        </p>
      )}
    </div>
  );
}

function MemoryBar({ total, segments }: { total: number; segments: { label: string; value: number; color: string; available?: boolean; untracked?: boolean }[] }) {
  const visible = segments.filter((segment) => segment.available !== false && segment.value > 0);
  return (
    <div className="memory-bar" aria-label="内存占比条">
      {visible.length === 0 ? <span className="memory-bar-empty">—</span> : visible.map((segment) => (
        <span
          key={segment.label}
          className={segment.untracked ? 'memory-bar-segment is-untracked' : 'memory-bar-segment'}
          title={`${segment.label}: ${formatMB1(segment.value)}`}
          style={{ width: `${total > 0 ? (segment.value / total) * 100 : 0}%`, backgroundColor: segment.color }}
        />
      ))}
    </div>
  );
}

function MemoryValueRow({ label, color, available, used, reserved, count }: { label: string; color?: string; available: boolean; used: number; reserved?: number; count?: number }) {
  return (
    <div className={`memory-value-row ${available ? '' : 'is-unavailable'}`}>
      <span>{color && <span className="swatch" style={{ background: color }} />}{label}</span>
      <span className="mono">{available ? (count == null ? (reserved == null ? formatMB1(used) : `${formatMB1(used)} / ${formatMB1(reserved)}`) : `${formatCount(count)} / ${formatGCAlloc(used)}`) : '—'}</span>
    </div>
  );
}

function CounterTable({ rows }: { rows: { label: string; value: string }[] }) {
  return (
    <table className="profiler-table">
      <thead>
        <tr>
          <th>Counter</th>
          <th>Value</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.label}>
            <td>{r.label}</td>
            <td className="mono">{r.value}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
