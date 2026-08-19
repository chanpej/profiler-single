import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import {
  AssistantRuntimeProvider,
  AuiIf,
  ComposerPrimitive,
  MessagePrimitive,
  ThreadPrimitive,
  useLocalRuntime,
  type TextMessagePartProps,
} from '@assistant-ui/react';
import { ArrowUp, Bot, Download, LoaderCircle, PanelRightClose, PanelRightOpen, Wrench } from 'lucide-react';
import '../styles/agentChat.css';
import { createAgentChatAdapter, type AgentChatTurnRecord } from '../lib/agentChatAdapter';
import { parseAgentChatContent } from '../lib/agentChatContent';
import { agentChatExportFilename, formatAgentChatSession } from '../lib/agentChatExport';
import { COLLAPSED_AGENT_WIDTH, saveAgentWidth, widthFromDrag } from '../lib/agentPanel';
import { activeAgent, isAgentConfigured } from '../lib/llmConfig';
import { contextMenuFromEvent, type ChatContextMenuState } from '../lib/agentChatUi';
import type { ModelOption } from '../lib/agentRuntime';
import type { AgentProfile, LLMConfig } from '../types';
import { ChatContextMenu } from './ChatContextMenu';
import { ComposerToolbar } from './ComposerToolbar';

function UserMessageText({ text }: TextMessagePartProps) {
  return <div className="agent-chat-message-text">{text}</div>;
}

function AssistantMessageText({ text }: TextMessagePartProps) {
  const segments = useMemo(() => parseAgentChatContent(text), [text]);
  return (
    <div className="agent-chat-message-text">
      {segments.map((segment, index) => (
        segment.type === 'text' ? (
          <div className="agent-chat-text-segment" key={`text-${index}`}>{segment.text}</div>
        ) : (
          <details className="agent-chat-tool-group" key={`tool-${index}`}>
            <summary>
              <Wrench size={12} aria-hidden />
              MCP 工具调用
              <span>{segment.entries.length} 条进度</span>
            </summary>
            <div className="agent-chat-tool-entries">
              {segment.entries.map((entry, entryIndex) => <div key={`${entryIndex}-${entry}`}>{entry}</div>)}
            </div>
          </details>
        )
      ))}
    </div>
  );
}

const userMessagePartComponents = { Text: UserMessageText };
const assistantMessagePartComponents = { Text: AssistantMessageText };

interface Props {
  llm: LLMConfig;
  collapsed: boolean;
  width: number;
  onToggle: () => void;
  onWidthChange: (width: number) => void;
  onSelectAgent: (id: string) => void;
  onUpdateAgent: (patch: Partial<AgentProfile> & { id: string }) => void;
  listModels?: (cfg: LLMConfig) => Promise<ModelOption[]>;
  agentChatSend: (message: string) => Promise<string>;
  onChunk: (handler: (text: string) => void) => () => void;
  exportSession?: (filename: string, content: string) => Promise<void>;
}

const AgentMessage = memo(function AgentMessage({ role }: { role: 'user' | 'assistant' }) {
  return (
    <MessagePrimitive.Root className={`agent-chat-message agent-chat-message-${role}`}>
      <MessagePrimitive.Parts components={role === 'assistant' ? assistantMessagePartComponents : userMessagePartComponents} />
      <AuiIf condition={(state) => state.message.status?.type === 'running'}>
        <span className="agent-chat-loading" role="status">
          <LoaderCircle size={14} aria-hidden />
          Agent 正在分析性能数据…
        </span>
      </AuiIf>
      <MessagePrimitive.Error>
        <span className="agent-chat-error">请求失败，请检查 Agent 配置后重试。</span>
      </MessagePrimitive.Error>
    </MessagePrimitive.Root>
  );
});

function AgentChatThread({
  agentName,
  agentChatSend,
  onChunk,
  onTurnUpdate,
  toolbar,
}: {
  agentName: string;
  agentChatSend: Props['agentChatSend'];
  onChunk: Props['onChunk'];
  onTurnUpdate: (turn: AgentChatTurnRecord) => void;
  toolbar: React.ReactNode;
}) {
  const adapter = useMemo(
    () => createAgentChatAdapter({ send: agentChatSend, onChunk, onTurnUpdate }),
    [agentChatSend, onChunk, onTurnUpdate],
  );
  const runtime = useLocalRuntime(adapter);

  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <ThreadPrimitive.Root className="agent-chat-thread" data-testid="assistant-ui-chat">
        <ThreadPrimitive.Viewport className="agent-chat-viewport" autoScroll>
          <div className="agent-chat-messages">
            <AuiIf condition={(state) => state.thread.isEmpty}>
              <div className="agent-chat-welcome">
                <span className="agent-chat-welcome-icon"><Bot size={28} aria-hidden /></span>
                <strong>{agentName || 'Perf Agent'}</strong>
                <p>Unity 2022 性能分析</p>
              </div>
            </AuiIf>
            <ThreadPrimitive.Messages>
              {({ message }) => (
                message.role === 'user'
                  ? <AgentMessage role="user" />
                  : message.role === 'assistant'
                    ? <AgentMessage role="assistant" />
                    : null
              )}
            </ThreadPrimitive.Messages>
          </div>
        </ThreadPrimitive.Viewport>
        <div className="agent-chat-footer">
          <ComposerPrimitive.Root className="agent-composer">
            <ComposerPrimitive.Input
              className="agent-composer-input"
              aria-label="询问性能问题…"
              placeholder="询问性能问题…"
              submitMode="enter"
            />
            <ComposerPrimitive.Send className="agent-composer-send" aria-label="发送" title="发送">
              <ArrowUp size={16} aria-hidden />
            </ComposerPrimitive.Send>
          </ComposerPrimitive.Root>
          {toolbar}
        </div>
      </ThreadPrimitive.Root>
    </AssistantRuntimeProvider>
  );
}

/** 右侧 ACP Agent 聊天面板：assistant-ui 会话 UI，可拖拽宽度、可收起。 */
export function AgentChatPanel({
  llm, collapsed, width, onToggle, onWidthChange, onSelectAgent, onUpdateAgent, listModels, agentChatSend, onChunk, exportSession,
}: Props) {
  const drag = useRef<{ startX: number; startW: number } | null>(null);
  const widthRef = useRef(width);
  widthRef.current = width;
  const [contextMenu, setContextMenu] = useState<ChatContextMenuState | null>(null);
  const llmRef = useRef(llm);
  llmRef.current = llm;
  const [discoveredModels, setDiscoveredModels] = useState<ModelOption[]>([]);
  const [turns, setTurns] = useState<AgentChatTurnRecord[]>([]);
  const [exporting, setExporting] = useState(false);
  const ready = isAgentConfigured(llm);
  const current = activeAgent(llm);

  const onTurnUpdate = useCallback((turn: AgentChatTurnRecord) => {
    setTurns((existing) => {
      const index = existing.findIndex((item) => item.id === turn.id);
      if (index < 0) return [...existing, turn];
      const next = existing.slice();
      next[index] = turn;
      return next;
    });
  }, []);

  useEffect(() => {
    setTurns([]);
  }, [llm.activeId]);

  const onExportSession = useCallback(async () => {
    if (!exportSession || turns.length === 0) return;
    const now = new Date();
    setExporting(true);
    try {
      await exportSession(
        agentChatExportFilename(now),
        formatAgentChatSession(current.name, turns, now),
      );
    } finally {
      setExporting(false);
    }
  }, [current.name, exportSession, turns]);

  useEffect(() => {
    if (!listModels || !(current.command ?? '').trim()) {
      setDiscoveredModels([]);
      return;
    }
    if (collapsed) return;
    let cancelled = false;
    listModels(llmRef.current).then((rows) => {
      if (!cancelled) setDiscoveredModels(rows ?? []);
    }).catch(() => {
      if (!cancelled) setDiscoveredModels([]);
    });
    return () => { cancelled = true; };
  }, [collapsed, current.id, current.command, listModels]);

  const onResizePointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.currentTarget.setPointerCapture(e.pointerId);
    drag.current = { startX: e.clientX, startW: width };
  }, [width]);

  const onResizePointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!drag.current) return;
    onWidthChange(widthFromDrag(drag.current.startW, drag.current.startX, e.clientX));
  }, [onWidthChange]);

  const onResizePointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!drag.current) return;
    drag.current = null;
    try {
      e.currentTarget.releasePointerCapture(e.pointerId);
    } catch {
      // capture 可能已释放
    }
    saveAgentWidth(widthRef.current);
  }, []);

  const onChatContextMenu = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    const next = contextMenuFromEvent(e.nativeEvent);
    if (!next) return;
    e.preventDefault();
    e.stopPropagation();
    setContextMenu(next);
  }, []);

  const panelWidth = collapsed ? COLLAPSED_AGENT_WIDTH : width;
  const toolbar = (
    <ComposerToolbar
      agents={llm.agents}
      agent={current}
      models={discoveredModels}
      onSelectAgent={onSelectAgent}
      onUpdateAgent={onUpdateAgent}
    />
  );

  return (
    <aside
      className={`agent-panel ${collapsed ? 'is-collapsed' : ''}`}
      style={{ width: panelWidth }}
      data-testid="agent-panel"
      aria-label="Agent 聊天"
    >
      {!collapsed && (
        <div
          className="agent-resize"
          role="separator"
          aria-orientation="vertical"
          aria-label="调整 Agent 面板宽度"
          data-testid="agent-resize"
          onPointerDown={onResizePointerDown}
          onPointerMove={onResizePointerMove}
          onPointerUp={onResizePointerUp}
          onPointerCancel={onResizePointerUp}
        />
      )}
      <div className="agent-panel-head">
        <Bot size={14} aria-hidden />
        {!collapsed && <span className="agent-panel-title">{current.name || 'Agent'}</span>}
        {!collapsed && (
          <button
            type="button"
            className="btn ghost"
            onClick={() => { void onExportSession(); }}
            title="导出整个 Agent Chat 会话"
            aria-label="导出 Agent Chat 会话"
            disabled={!exportSession || turns.length === 0 || exporting}
          >
            <Download size={14} aria-hidden />
          </button>
        )}
        <button
          type="button"
          className="btn ghost"
          onClick={onToggle}
          title={collapsed ? '展开 Agent 面板' : '收起 Agent 面板'}
          aria-label={collapsed ? '展开 Agent 面板' : '收起 Agent 面板'}
          aria-expanded={!collapsed}
        >
          {collapsed ? <PanelRightOpen size={14} aria-hidden /> : <PanelRightClose size={14} aria-hidden />}
        </button>
      </div>
      <div
        className="agent-chat-body"
        hidden={collapsed}
        data-testid="agent-chat-body"
        onContextMenu={onChatContextMenu}
      >
        {ready ? (
          <AgentChatThread
            key={llm.activeId}
            agentName={current.name}
            agentChatSend={agentChatSend}
            onChunk={onChunk}
            onTurnUpdate={onTurnUpdate}
            toolbar={toolbar}
          />
        ) : (
          <p className="dim agent-chat-hint">请先在设置中配置 ACP Agent。</p>
        )}
        {contextMenu && createPortal(
          <ChatContextMenu menu={contextMenu} onClose={() => setContextMenu(null)} />,
          document.body,
        )}
      </div>
    </aside>
  );
}
