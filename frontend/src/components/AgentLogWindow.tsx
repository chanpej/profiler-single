import { useCallback, useEffect, useState } from 'react';
import { RefreshCw, Trash2 } from 'lucide-react';
import { api } from '../api';
import type { AgentLogEntry } from '../types';
import '../styles/agentLog.css';

function formatTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString();
}

/** 独立原生窗口使用的 Agent/MCP 完整交互记录。 */
export function AgentLogWindow() {
  const [logs, setLogs] = useState<AgentLogEntry[]>([]);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setLogs(await api.agentLogs());
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh().catch(() => {});
    const timer = window.setInterval(() => { refresh().catch(() => {}); }, 1000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const clear = async () => {
    await api.clearAgentLogs();
    setLogs([]);
  };

  return (
    <main className="agent-log-window">
      <header className="agent-log-head">
        <div>
          <h1>Agent Log</h1>
          <p>Agent 聊天与嵌入式 MCP Server 的完整交互记录（最多保留 1000 条）。</p>
        </div>
        <div className="agent-log-actions">
          <button type="button" className="btn" onClick={() => refresh().catch(() => {})} disabled={loading}>
            <RefreshCw size={14} aria-hidden /> 刷新
          </button>
          <button type="button" className="btn" onClick={() => clear().catch(() => {})} disabled={logs.length === 0}>
            <Trash2 size={14} aria-hidden /> 清空
          </button>
        </div>
      </header>
      <section className="agent-log-list" aria-label="Agent 交互日志">
        {logs.length === 0 ? (
          <p className="dim agent-log-empty">尚无记录。向 Agent 发送消息后，MCP 工具调用会显示在这里。</p>
        ) : logs.map((entry, index) => (
          <article className="agent-log-entry" key={`${entry.time}-${index}`}>
            <header>
              <span className={`agent-log-source ${entry.source === 'MCP' ? 'is-mcp' : ''}`}>{entry.source}</span>
              <strong>{entry.direction}</strong>
              <span className="mono">{entry.method}</span>
              <time className="dim">{formatTime(entry.time)}</time>
            </header>
            <pre>{entry.payload}</pre>
          </article>
        ))}
      </section>
    </main>
  );
}
