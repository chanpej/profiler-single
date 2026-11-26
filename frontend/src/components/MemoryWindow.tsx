import { useEffect, useState } from 'react';
import { MemorySnapshotLibrary } from '../../bindings/unityperf/services/perfservice';

const toMiB = (bytes: number) => (bytes / 1048576).toLocaleString(undefined, { maximumFractionDigits: 2 });
export function MemoryWindow() {
  const [library, setLibrary] = useState<Awaited<ReturnType<typeof MemorySnapshotLibrary>>>();
  const [selection, select] = useState('');
  const [error, setError] = useState('');
  useEffect(() => {
    let active = true;
    let loading = false;
    const refresh = async () => {
      if (loading) return;
      loading = true;
      try { const data = await MemorySnapshotLibrary(); if (active) setLibrary(data); }
      catch (reason) { if (active) setError(String(reason)); }
      finally { loading = false; }
    };
    void refresh();
    const timer = window.setInterval(refresh, 4000);
    window.addEventListener('focus', refresh);
    return () => { active = false; window.clearInterval(timer); window.removeEventListener('focus', refresh); };
  }, []);
  const records = library?.records ?? [];
  const visible = records;
  const selected = visible.find(item => item.id === selection) ?? visible[0];
  const rows = selected?.sample.rows ?? [];
  return <main className="memory-library"><header><h1>Memory Profile</h1></header>
    {error && <p role="alert">{error}</p>}
    <div className="memory-library-columns"><aside><nav>当前会话 ({visible.length})</nav>
      {visible.map(item=><button type="button" className="memory-record" key={item.id} aria-pressed={item.id===selected?.id} onClick={()=>select(item.id)}><strong>{item.sample.target}</strong><small>{new Date(item.time).toLocaleString()} · {item.sample.source==='snap'?'Unity 快照':'Android PSS'}</small></button>)}
      {!visible.length && <p>当前会话暂无快照。可在主窗口画面轨道点击 Memory Profiler Capture。</p>}
    </aside><article>{selected && <><h2>{selected.sample.target}</h2><p>{selected.sample.source==='snap'?'原生对象合计':'Android 进程 PSS'}　{toMiB(rows.reduce((sum,r)=>sum+r.bytes,0))} MiB</p>{selected.path && <p className="dim">{selected.path}</p>}
      <table><thead><tr><th>类型 / 分类</th>{selected.sample.source==='snap' && <th>对象数</th>}<th>大小 (MiB)</th></tr></thead><tbody>{rows.map((r,i)=><tr key={i}><td>{r.name}</td>{selected.sample.source==='snap' && <td>{r.count}</td>}<td>{toMiB(r.bytes)}</td></tr>)}</tbody></table></>}</article></div></main>;
}
