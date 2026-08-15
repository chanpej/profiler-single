import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Bot, PanelRightClose, PanelRightOpen } from 'lucide-react';
import { AiChat, useAsStreamAdapter } from '@nlux/react';
import '@nlux/themes/nova.css';
import '../styles/agentChat.css';
import { bindAgentStream } from '../lib/agentChatAdapter';
import { COLLAPSED_AGENT_WIDTH, saveAgentWidth, widthFromDrag } from '../lib/agentPanel';
import { activeAgent, isAgentConfigured } from '../lib/llmConfig';
import { clearComposerInput, contextMenuFromEvent, watchComposerSubmitClear, type ChatContextMenuState } from '../lib/agentChatUi';
import type { ModelOption } from '../lib/agentRuntime';
import type { AgentProfile, LLMConfig } from '../types';
import { ChatContextMenu } from './ChatContextMenu';
import { ComposerToolbar } from './ComposerToolbar';

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
}

/** 右侧 ACP Agent 聊天面板：NLUX 会话 UI，可拖拽宽度、可收起。 */
export function AgentChatPanel({
  llm, collapsed, width, onToggle, onWidthChange, onSelectAgent, onUpdateAgent, listModels, agentChatSend, onChunk,
}: Props) {
  const drag = useRef<{ startX: number; startW: number } | null>(null);
  const widthRef = useRef(width);
  widthRef.current = width;
  const bodyRef = useRef<HTMLDivElement>(null);
  const [composerSlot, setComposerSlot] = useState<HTMLElement | null>(null);
  const [contextMenu, setContextMenu] = useState<ChatContextMenuState | null>(null);
  const llmRef = useRef(llm);
  llmRef.current = llm;
  const [discoveredModels, setDiscoveredModels] = useState<ModelOption[]>([]);
  const ready = isAgentConfigured(llm);
  const current = activeAgent(llm);

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

  const streamSend = useMemo(
    () => bindAgentStream(agentChatSend, onChunk),
    [agentChatSend, onChunk],
  );
  const adapter = useAsStreamAdapter((message, observer) => {
    streamSend(message, observer);
  }, [streamSend]);

  useEffect(() => {
    const root = bodyRef.current;
    if (!root || collapsed || !ready) {
      setComposerSlot(null);
      return;
    }
    const mount = () => {
      const container = root.querySelector('.nlux-composer-container');
      if (!container) return false;
      let slot = container.querySelector('.agent-composer-toolbar-slot') as HTMLElement | null;
      if (!slot) {
        slot = document.createElement('div');
        slot.className = 'agent-composer-toolbar-slot';
        container.appendChild(slot);
      }
      setComposerSlot(slot);
      return true;
    };
    if (mount()) return;
    const obs = new MutationObserver(() => {
      if (mount()) obs.disconnect();
    });
    obs.observe(root, { childList: true, subtree: true });
    return () => obs.disconnect();
  }, [collapsed, ready, llm.activeId]);

  useEffect(() => {
    const root = bodyRef.current;
    if (!root || collapsed || !ready) return;
    return watchComposerSubmitClear(root);
  }, [collapsed, ready, llm.activeId]);

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
        ref={bodyRef}
        onContextMenu={onChatContextMenu}
      >
        {ready ? (
          <AiChat
            key={llm.activeId}
            adapter={adapter}
            displayOptions={{ colorScheme: 'dark', height: '100%' }}
            composerOptions={{ placeholder: '询问性能问题…' }}
            conversationOptions={{ historyPayloadSize: 'max', showWelcomeMessage: true }}
            events={{
              messageSent: () => { clearComposerInput(bodyRef.current); },
            }}
            personaOptions={{
              assistant: {
                name: current.name || 'Perf Agent',
                tagline: 'Unity 2022 性能分析',
                avatar: <Bot size={16} />,
              },
            }}
          />
        ) : (
          <p className="dim agent-chat-hint">请先在设置中配置 ACP Agent。</p>
        )}
        {composerSlot
          ? createPortal(toolbar, composerSlot)
          : (!collapsed ? toolbar : null)}
        {contextMenu && createPortal(
          <ChatContextMenu menu={contextMenu} onClose={() => setContextMenu(null)} />,
          document.body,
        )}
      </div>
    </aside>
  );
}
