import { useEffect, useMemo, useRef, useState } from 'react';

interface PlayerLog {
  id: number;
  receivedAt: number;
  level: string;
  text: string;
}

const maxEntries = 500;
const logLevels = ['Log', 'Warning', 'Error'] as const;
const previewLineOptions = Array.from({ length: 10 }, (_, index) => index + 1);

function filterLevel(level: string): (typeof logLevels)[number] {
  switch (level) {
    case 'Warning': return 'Warning';
    case 'Error':
    case 'Assert':
    case 'Exception': return 'Error';
    default: return 'Log';
  }
}

export function UnityLogView({ fetchLogs, miniGame = false }: {
  fetchLogs: (after: number) => Promise<PlayerLog[]>;
  miniGame?: boolean;
}) {
  const [logs, setLogs] = useState<PlayerLog[]>([]);
  const [readError, setReadError] = useState(false);
  const [levels, setLevels] = useState({ Log: true, Warning: true, Error: true });
  const [search, setSearch] = useState('');
  const [showTimestamp, setShowTimestamp] = useState(true);
  const [previewLines, setPreviewLines] = useState(1);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const list = useRef<HTMLDivElement>(null);
  const detail = useRef<HTMLPreElement>(null);
  const follow = useRef(true);
  const visibleLogs = useMemo(() => {
    const query = search.trim().toLowerCase();
    return logs.filter((log) => levels[filterLevel(log.level)] &&
      (!query || log.text.toLowerCase().includes(query)));
  }, [levels, logs, search]);
  const selectedLog = visibleLogs.find((log) => log.id === selectedId);

  useEffect(() => {
    let active = true;
    let after = 0;
    let timer: number;
    const refresh = async () => {
      try {
        const batch = await fetchLogs(after);
        if (!active) return;
        setReadError(false);
        if (batch.length > 0) {
          after = batch[batch.length - 1].id;
          setLogs((previous) => [...previous, ...batch].slice(-maxEntries));
        }
      } catch {
        if (active) setReadError(true);
      } finally {
        if (active) timer = window.setTimeout(refresh, 500);
      }
    };
    void refresh();
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [fetchLogs]);

  useEffect(() => {
    if (follow.current && list.current) list.current.scrollTop = list.current.scrollHeight;
  }, [visibleLogs, previewLines, showTimestamp]);

  useEffect(() => {
    if (detail.current) detail.current.scrollTop = 0;
  }, [selectedLog]);

  return (
    <div className="unity-log-view">
      <div className="unity-log-filters">
        <div className="unity-log-levels" role="group" aria-label="日志级别">
          {logLevels.map((level) => (
            <button key={level} type="button" className="btn" aria-pressed={levels[level]}
              onClick={() => setLevels((previous) => ({ ...previous, [level]: !previous[level] }))}>
              {level}
            </button>
          ))}
        </div>
        <input className="input" type="search" aria-label="搜索日志" placeholder="搜索日志"
          value={search} onChange={(event) => setSearch(event.target.value)} />
        <button type="button" className="btn" aria-pressed={showTimestamp}
          onClick={() => setShowTimestamp((previous) => !previous)}>
          时间戳
        </button>
        <select className="input unity-log-line-count" aria-label="每条日志预览行数" value={previewLines}
          onChange={(event) => setPreviewLines(Number(event.target.value))}>
          {previewLineOptions.map((lines) => <option key={lines} value={lines}>每条 {lines} 行</option>)}
        </select>
      </div>
      <div className="unity-log-list mono" ref={list} onScroll={(event) => {
        const element = event.currentTarget;
        follow.current = element.scrollHeight - element.scrollTop - element.clientHeight < 24;
      }}>
        {readError && <p role="status">读取 Unity 日志失败，正在重试。</p>}
        {!readError && logs.length === 0 && (
          <p className="dim">{miniGame
            ? '等待小游戏输出 Unity 日志。'
            : '等待 Unity Player 输出日志。'}</p>
        )}
        {logs.length > 0 && visibleLogs.length === 0 && (
          <p className="dim">没有符合当前筛选条件的日志。</p>
        )}
        {visibleLogs.map((log) => (
          <button key={log.id} type="button" className="unity-log-entry" data-level={log.level}
            aria-pressed={log.id === selectedId} onClick={() => {
              follow.current = false;
              setSelectedId(log.id);
            }}>
            <span className="unity-log-meta">
              {showTimestamp && <time className="dim">{new Date(log.receivedAt).toLocaleTimeString()}</time>}
              <span className="unity-log-level">{log.level}</span>
            </span>
            <span className="unity-log-preview" style={{ WebkitLineClamp: previewLines }}>
              {log.text.split(/\r\n?|\n/, previewLines).join('\n')}
            </span>
          </button>
        ))}
      </div>
      <section className="unity-log-detail mono" aria-label="日志详情">
        {selectedLog ? <pre ref={detail} tabIndex={0}>{selectedLog.text}</pre>
          : <p className="dim">选中一条日志查看完整内容。</p>}
      </section>
    </div>
  );
}
