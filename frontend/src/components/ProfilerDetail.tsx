import { useMemo, useRef, useState } from 'react';
import type { FrameData, ProfilerModuleId, SampleNode } from '../types';
import { memorySimpleRows, moduleBreakdown, moduleById, renderingCounters, frameTimeMs, gpuOrFrameMs } from '../lib/profiler';
import { formatCount, formatFps, formatMB1, formatMs, formatPct, formatProfilerValue } from '../lib/format';
import { categoryColor, flattenVisible, mergeExpanded } from '../lib/hierarchy';

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
          <CpuTimeline frame={frame} />
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
  const touched = useRef(new Set<string>());
  const expandOpts = { revealCallstacks: callStacks };
  const [expanded, setExpanded] = useState(() => mergeExpanded(new Set(), nodes, touched.current, expandOpts));
  const [selected, setSelected] = useState<string>('');

  const visibleExpanded = useMemo(
    () => mergeExpanded(expanded, nodes, touched.current, { revealCallstacks: callStacks }),
    [expanded, nodes, callStacks],
  );
  const rows = useMemo(() => flattenVisible(nodes, visibleExpanded), [nodes, visibleExpanded]);
  const selectedRow = rows.find((r) => r.path === selected);
  const stack = selectedRow?.node.callstack ?? [];
  // 本帧是否真的带调用栈：Player 只在发生分配的帧写调用栈，空面板要能说清是哪种"空"。
  const frameHasStack = useMemo(() => rows.some((r) => (r.node.callstack?.length ?? 0) > 0), [rows]);

  return (
    <div className="sample-hierarchy" data-testid="cpu-hierarchy">
      <table className="profiler-table">
        <thead>
          <tr>
            <th>Overview</th>
            <th>Total</th>
            <th>Self</th>
            <th>Calls</th>
            <th>GC Alloc</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr
              key={r.path}
              className={selected === r.path ? 'is-selected' : ''}
              onClick={() => setSelected(r.path)}
            >
              <td>
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
                          const next = new Set(mergeExpanded(cur, nodes, touched.current, { revealCallstacks: callStacks }));
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
                  {r.node.name}
                </span>
              </td>
              <td className="mono">{formatMs(r.node.totalMs)}</td>
              <td className="mono">{formatMs(r.node.selfMs)}</td>
              <td className="mono">{r.node.calls}</td>
              <td className="mono">{r.node.gcAllocKB > 0 ? `${r.node.gcAllocKB.toFixed(1)} KB` : '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {callStacks && (
        <div className="callstack-pane" data-testid="callstack-pane">
          <h3>Call Stacks</h3>
          {stack.length > 0 ? (
            <pre className="callstack-list">{stack.join('\n')}</pre>
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

function CpuTimeline({ frame }: { frame: FrameData }) {
  const cats = moduleBreakdown(moduleById('cpu'), frame);
  const total = cats.reduce((s, c) => s + c.value, 0) || 1;
  return (
    <div className="cpu-timeline" data-testid="cpu-timeline">
      <div className="cpu-timeline-row">
        <span className="cpu-timeline-label">Main Thread</span>
        <div className="cpu-timeline-bar" role="img" aria-label="Main Thread 时间线">
          {cats.map((c) => (
            <span
              key={c.id}
              className="cpu-timeline-seg"
              style={{ width: `${(c.value / total) * 100}%`, background: c.color }}
              title={`${c.label} ${formatMs(c.value)}`}
            />
          ))}
        </div>
        <span className="mono dim">{formatMs(frame.mainThreadMs)}</span>
      </div>
      <div className="cpu-timeline-row">
        <span className="cpu-timeline-label">Render Thread</span>
        <div className="cpu-timeline-bar">
          <span className="cpu-timeline-seg" style={{ width: '100%', background: '#2E8B4A' }} title={`Render ${formatMs(frame.renderMs)}`} />
        </div>
        <span className="mono dim">{formatMs(frame.renderMs)}</span>
      </div>
    </div>
  );
}
