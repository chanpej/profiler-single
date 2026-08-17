import { useCallback, useMemo, useRef, useState } from 'react';
import { ClipboardCopy } from 'lucide-react';
import type { FrameData, ProfilerModuleId, SampleNode } from '../types';
import { memorySimpleRows, moduleBreakdown, moduleById, renderingCounters, frameTimeMs, gpuOrFrameMs } from '../lib/profiler';
import { formatCount, formatFps, formatMB1, formatMs, formatPct, formatProfilerValue } from '../lib/format';
import { categoryColor, flattenVisible, mergeExpanded, mergeSampleNodes } from '../lib/hierarchy';
import { copyText } from '../lib/agentChatUi';
import { CpuTimeline } from './CpuTimeline';
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
}

type CpuTab = 'hierarchy' | 'timeline';
type MemTab = 'simple' | 'detailed';

/** 选中帧的模块详情：还原 Unity Profiler 底部面板。 */
export function ProfilerDetail({ moduleId, frame, hierarchy = [], callStacks = false }: Props) {
  const [cpuTab, setCpuTab] = useState<CpuTab>('hierarchy');
  const [memTab, setMemTab] = useState<MemTab>('simple');
  const mod = moduleById(moduleId);

  return (
    <div className="profiler-detail" data-testid="profiler-detail">
      <div className="profiler-detail-head">
        <h2>{mod.label}</h2>
        {moduleId === 'cpu' && (
          <div className="profiler-tabs" role="tablist" aria-label="CPU 视图">
            <button type="button" role="tab" aria-selected={cpuTab === 'hierarchy'} className={cpuTab === 'hierarchy' ? 'is-active' : ''} onClick={() => setCpuTab('hierarchy')}>Hierarchy</button>
            <button type="button" role="tab" aria-selected={cpuTab === 'timeline'} className={cpuTab === 'timeline' ? 'is-active' : ''} onClick={() => setCpuTab('timeline')}>Timeline</button>
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
        {!frame ? (
          <p className="dim">选择一帧以查看详情</p>
        ) : moduleId === 'cpu' && cpuTab === 'timeline' ? (
          <CpuTimeline frame={frame} hierarchy={hierarchy} />
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
  const metricValues = useMemo(
    () => rows.map((r) => [
      formatMs(r.node.totalMs),
      formatMs(r.node.selfMs),
      String(r.node.calls),
      r.node.gcAllocKB > 0 ? `${r.node.gcAllocKB.toFixed(1)} KB` : '—',
    ]),
    [rows],
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
                <th className="col-metric">Total</th>
                <th className="col-metric">Self</th>
                <th className="col-metric">Calls</th>
                <th className="col-metric">GC Alloc</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
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
                      <span className="swatch" style={{ background: categoryColor(r.node.category) }} />
                      <span className="hierarchy-name" title={r.node.name}>{r.node.name}</span>
                    </span>
                  </td>
                  <td className="mono col-metric">{formatMs(r.node.totalMs)}</td>
                  <td className="mono col-metric">{formatMs(r.node.selfMs)}</td>
                  <td className="mono col-metric">{r.node.calls}</td>
                  <td className="mono col-metric">{r.node.gcAllocKB > 0 ? `${r.node.gcAllocKB.toFixed(1)} KB` : '—'}</td>
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
  return (
    <table className="profiler-table" data-testid="memory-simple">
      <thead>
        <tr>
          <th>Category</th>
          <th>Used</th>
          <th>Reserved</th>
        </tr>
      </thead>
      <tbody>
        {memorySimpleRows(frame).map((r) => (
          <tr key={r.label}>
            <td>{r.label}</td>
            <td className="mono">{formatMB1(r.used)}</td>
            <td className="mono">{r.reserved > 0 ? formatMB1(r.reserved) : '—'}</td>
          </tr>
        ))}
        <tr>
          <td>GC Alloc</td>
          <td className="mono">{frame.gcAllocKB.toFixed(1)} KB</td>
          <td className="mono">—</td>
        </tr>
      </tbody>
    </table>
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
