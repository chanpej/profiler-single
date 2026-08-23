import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import {
  AssistantRuntimeProvider,
  AuiIf,
  ComposerPrimitive,
  MessagePrimitive,
  ThreadPrimitive,
  unstable_useComposerInput,
  useLocalRuntime,
  type TextMessagePartProps,
} from '@assistant-ui/react';
import { ArrowUp, Bot, Box, Download, Layers, LoaderCircle, PanelRightClose, PanelRightOpen, Wrench } from 'lucide-react';
import '../styles/agentChat.css';
import { createAgentChatAdapter, type AgentChatTurnRecord } from '../lib/agentChatAdapter';
import { parseAgentChatContent } from '../lib/agentChatContent';
import { agentChatExportFilename, formatAgentChatSession } from '../lib/agentChatExport';
import { COLLAPSED_AGENT_WIDTH, saveAgentWidth, widthFromDrag } from '../lib/agentPanel';
import { activeAgent, isAgentConfigured } from '../lib/llmConfig';
import { contextMenuFromEvent, type ChatContextMenuState } from '../lib/agentChatUi';
import { formatAgentChatReference, type AgentChatReference } from '../lib/agentChatReferences';
import type { ModelOption } from '../lib/agentRuntime';
import type { AgentProfile, LLMConfig } from '../types';
import { ChatContextMenu } from './ChatContextMenu';
import { ComposerToolbar } from './ComposerToolbar';

function UserMessageText({ text }: TextMessagePartProps) {
  const segments = useMemo(() => parseAgentChatContent(text), [text]);
  return <MessageContent segments={segments} />;
}

function AssistantMessageText({ text }: TextMessagePartProps) {
  const segments = useMemo(() => parseAgentChatContent(text), [text]);
  return <MessageContent segments={segments} />;
}

function MessageContent({ segments }: { segments: ReturnType<typeof parseAgentChatContent> }) {
  return (
    <div className="agent-chat-message-text">
      {segments.map((segment, index) => (
        segment.type === 'text' ? (
          <span className="agent-chat-text-segment" key={`text-${index}`}>{segment.text}</span>
        ) : segment.type === 'reference' ? (
          <span className={`agent-chat-reference agent-chat-reference-${segment.kind} agent-chat-reference-${segment.referenceType}`} key={`reference-${index}`} title={segment.description}>
            {segment.kind === 'module' ? <Box size={13} aria-hidden /> : <Layers size={13} aria-hidden />}
            <strong>{segment.label}</strong>
          </span>
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
  pendingReference?: { id: number; reference: AgentChatReference } | null;
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
  pendingReference,
}: {
  agentName: string;
  agentChatSend: Props['agentChatSend'];
  onChunk: Props['onChunk'];
  onTurnUpdate: (turn: AgentChatTurnRecord) => void;
  toolbar: React.ReactNode;
  pendingReference?: { id: number; reference: AgentChatReference } | null;
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
            <AgentComposer pendingReference={pendingReference} />
          </ComposerPrimitive.Root>
          {toolbar}
        </div>
      </ThreadPrimitive.Root>
    </AssistantRuntimeProvider>
  );
}

function AgentComposer({ pendingReference }: { pendingReference?: { id: number; reference: AgentChatReference } | null }) {
  const { setText, send, isDisabled } = unstable_useComposerInput();
  const editorRef = useRef<HTMLDivElement>(null);
  const referenceIDs = useRef(new Map<string, AgentChatReference>());
  const nextReferenceID = useRef(0);
  const lastRange = useRef<Range | null>(null);

  const rememberRange = useCallback(() => {
    const selection = window.getSelection();
    if (!selection?.rangeCount || !editorRef.current) return;
    const range = selection.getRangeAt(0);
    if (editorRef.current.contains(range.commonAncestorContainer)) lastRange.current = range.cloneRange();
  }, []);

  const serialized = useCallback(() => {
    const read = (nodes: NodeListOf<ChildNode> | ChildNode[]): string => Array.from(nodes).map((node) => {
      if (node.nodeType === Node.TEXT_NODE) return node.textContent ?? '';
      if (node.nodeType !== Node.ELEMENT_NODE) return '';
      const element = node as HTMLElement;
      if (element.dataset.referenceId) return formatAgentChatReference(referenceIDs.current.get(element.dataset.referenceId) ?? { kind: 'module', type: 'unknown', label: 'Unknown', description: '' });
      if (element.tagName === 'BR') return '\n';
      return read(element.childNodes);
    }).join('');
    return editorRef.current ? read(editorRef.current.childNodes) : '';
  }, []);

  const sync = useCallback(() => setText(serialized()), [serialized, setText]);

  const placeAfter = useCallback((node: Node) => {
    const range = document.createRange();
    range.setStartAfter(node);
    range.collapse(true);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    lastRange.current = range.cloneRange();
  }, []);

  const insertReference = useCallback((reference: AgentChatReference) => {
    const editor = editorRef.current;
    if (!editor) return;
    const id = `reference-${nextReferenceID.current++}`;
    referenceIDs.current.set(id, reference);
    const token = document.createElement('span');
    token.className = `agent-composer-reference agent-composer-reference-${reference.kind} agent-composer-reference-${reference.type}`;
    token.contentEditable = 'false';
    token.dataset.referenceId = id;
    token.title = reference.description;
    token.innerHTML = `${reference.kind === 'module' ? '▣' : '▤'} <span>${reference.label}</span><button type="button" aria-label="移除 ${reference.label} 引用" title="移除引用">×</button>`;
    const selection = window.getSelection();
    const range = selection?.rangeCount && editor.contains(selection.getRangeAt(0).commonAncestorContainer)
      ? selection.getRangeAt(0) : lastRange.current;
    if (range && editor.contains(range.commonAncestorContainer)) {
      range.deleteContents();
      range.insertNode(token);
    } else {
      editor.append(token);
    }
    placeAfter(token);
    editor.focus();
    sync();
  }, [placeAfter, sync]);

  useEffect(() => {
    if (!pendingReference) return;
    insertReference(pendingReference.reference);
  }, [insertReference, pendingReference?.id]);

  const submit = useCallback(() => {
    const message = serialized();
    if (!message.trim() || isDisabled) return;
    // setText 在 assistant-ui 中同 tick 同步，因此当前点击会发送包含引用的完整原文。
    setText(message);
    send();
    if (editorRef.current) editorRef.current.replaceChildren();
    referenceIDs.current.clear();
    lastRange.current = null;
    setText('');
  }, [isDisabled, send, serialized, setText]);

  return (
    <div className="agent-composer-rich">
      <div
        ref={editorRef}
        className="agent-composer-draft"
        contentEditable={!isDisabled}
        role="textbox"
        aria-multiline="true"
        aria-label="询问性能问题…"
        data-placeholder="询问性能问题…"
        onFocus={rememberRange}
        onKeyUp={rememberRange}
        onMouseUp={rememberRange}
        onInput={() => { rememberRange(); sync(); }}
        onClick={(event) => {
          const button = (event.target as HTMLElement).closest<HTMLButtonElement>('button[aria-label^="移除 "]');
          if (!button) return;
          const token = button.closest<HTMLElement>('[data-reference-id]');
          if (!token) return;
          referenceIDs.current.delete(token.dataset.referenceId ?? '');
          placeAfter(token);
          token.remove();
          sync();
        }}
        onKeyDown={(event) => {
          if (event.key === 'Enter' && !event.shiftKey) {
            event.preventDefault();
            submit();
          } else if (event.key === 'Enter') {
            event.preventDefault();
            const lineBreak = document.createElement('br');
            const selection = window.getSelection();
            const range = selection?.rangeCount ? selection.getRangeAt(0) : lastRange.current;
            if (range && editorRef.current?.contains(range.commonAncestorContainer)) {
              range.insertNode(lineBreak);
              placeAfter(lineBreak);
              sync();
            }
          }
        }}
      />
      <button type="button" className="agent-composer-send" aria-label="发送" title="发送" disabled={isDisabled} onClick={submit}>
        <ArrowUp size={16} aria-hidden />
      </button>
    </div>
  );
}

/** 右侧 ACP Agent 聊天面板：assistant-ui 会话 UI，可拖拽宽度、可收起。 */
export function AgentChatPanel({
  llm, collapsed, width, onToggle, onWidthChange, onSelectAgent, onUpdateAgent, listModels, agentChatSend, onChunk, exportSession, pendingReference,
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
            pendingReference={pendingReference}
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
