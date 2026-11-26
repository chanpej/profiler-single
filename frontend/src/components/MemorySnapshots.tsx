import { useEffect, useState } from 'react';

import type { OpenMemorySnapshot } from '../../bindings/unityperf/services/perfservice';
import { isUserCancelled } from '../lib/userCancel';

type Sample = Awaited<ReturnType<typeof OpenMemorySnapshot>>;
import { snapshotCaptured } from './MemoryCaptureButton';
const filters = [{ DisplayName: 'Unity Memory Profiler (*.snap)', Pattern: '*.snap' }];
const mib = (bytes: number) => (bytes / 1048576).toLocaleString(undefined, { maximumFractionDigits: 2 });

export function MemorySnapshots({ miniGame = false }: { miniGame?: boolean }) {
  const [current, setCurrent] = useState<Sample>();
  const [baseline, setBaseline] = useState<Sample>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [query, setQuery] = useState('');
  useEffect(() => {
    const received = (event: Event) => {
      setCurrent((event as CustomEvent<Sample>).detail);
      setBaseline(previous => previous?.source === 'snap' ? previous : undefined);
    };
    window.addEventListener(snapshotCaptured, received);
    return () => window.removeEventListener(snapshotCaptured, received);
  }, []);
  async function load(kind: 'open' | 'pss') {
    setBusy(true); setError('');
    try {
      const { CaptureAndroidMemory, OpenMemorySnapshot } = await import('../../bindings/unityperf/services/perfservice');
      let sample: Sample;
      if (kind === 'pss') sample = await CaptureAndroidMemory();
      else {
        const { Dialogs } = await import('@wailsio/runtime');
        const selected = await Dialogs.OpenFile({ Title: '打开 Unity 内存快照', Filters: filters, CanChooseFiles: true });
        const path = Array.isArray(selected) ? selected[0] : selected;
        if (!path) return;
        sample = await OpenMemorySnapshot(path);
      }
      setCurrent(sample);
      if (baseline && (baseline.source !== sample.source || (sample.source === 'pss' && baseline.target !== sample.target))) setBaseline(undefined);
    } catch (reason) { if (!isUserCancelled(reason)) setError(String(reason)); }
    finally { setBusy(false); }
  }
  const rows = current?.rows ?? [];
  const base = new Map((baseline?.rows ?? []).map(r => [r.name, r]));
  const present = new Set(rows.map(r => r.name));
  const combined = [...rows, ...(baseline?.rows ?? []).filter(r => !present.has(r.name)).map(r => ({ ...r, count: 0, bytes: 0 }))];
  const total = rows.reduce((sum,r) => sum+r.bytes, 0);
  return <section className="memory-snapshots" aria-label="内存快照">
    <div className="memory-snapshot-tools">
      <button className="btn" disabled={busy} onClick={() => load('open')}>打开 .snap</button>
      <button className="btn" disabled={busy} onClick={() => load('pss')}>采集前台进程 PSS</button>
      <button className="btn" disabled={!current || busy} onClick={() => setBaseline(current)}>设为对比基线</button>
    </div>
    {busy && <p role="status">正在读取内存数据…</p>}{error && <p role="alert">{error}</p>}
    {miniGame && <p>小游戏 Player 连接后可直接采集托管堆和原生对象，随当前会话保存到 .upac。</p>}
    {!miniGame && !current && <p>打开 Unity 2022 内存快照可查看原生对象类型、数量和大小。真机 .snap 采集需要在 Android 开发包中加入 UnityPerfMemoryCapture.cs。微信小游戏可采集前台宿主进程 PSS。</p>}
    {current && <><p>{current.target} · {current.source === 'snap' ? '原生对象合计' : 'Android 进程 PSS 分类合计'} {mib(total)} MiB</p>
      {baseline && <p>对比基线　{baseline.target}</p>}
      <input aria-label="筛选内存分类" placeholder="筛选类型或分类" value={query} onChange={e=>setQuery(e.target.value)} />
      <table><thead><tr><th>类型 / 分类</th>{current.source === 'snap' && <th>对象数</th>}<th>大小 (MiB)</th>{baseline && <th>相对基线 (MiB)</th>}</tr></thead><tbody>
      {combined.filter(r => r.name.toLowerCase().includes(query.toLowerCase())).map(r => <tr key={r.name}><td>{r.name}</td>{current.source === 'snap' && <td>{r.count}</td>}<td>{mib(r.bytes)}</td>{baseline && <td>{mib(r.bytes-(base.get(r.name)?.bytes ?? 0))}</td>}</tr>)}
      </tbody></table></>}
  </section>;
}
