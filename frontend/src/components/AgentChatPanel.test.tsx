import { describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ButtonHTMLAttributes, ComponentProps, HTMLAttributes, ReactNode, TextareaHTMLAttributes } from 'react';
import type { ChatModelAdapter } from '@assistant-ui/react';
import { AgentChatPanel } from './AgentChatPanel';
import { emptyLLMConfig, withActiveAgent } from '../lib/llmConfig';

const assistantUiMock = vi.hoisted(() => ({ adapter: null as ChatModelAdapter | null, setText: vi.fn(), send: vi.fn() }));

vi.mock('@assistant-ui/react', () => {
  const Wrap = ({ children, autoScroll: _autoScroll, ...props }: HTMLAttributes<HTMLDivElement> & { autoScroll?: boolean }) => <div {...props}>{children}</div>;
  return {
    AssistantRuntimeProvider: ({ children }: { children?: ReactNode }) => <>{children}</>,
    AuiIf: ({ children }: { children?: ReactNode }) => <>{children}</>,
    useLocalRuntime: (adapter: ChatModelAdapter) => {
      assistantUiMock.adapter = adapter;
      return {};
    },
    unstable_useComposerInput: () => ({ value: '', setText: assistantUiMock.setText, send: assistantUiMock.send, isDisabled: false }),
    ThreadPrimitive: {
      Root: Wrap,
      Viewport: Wrap,
      ViewportFooter: Wrap,
      Messages: ({ children }: { children: (value: { message: { role: 'assistant' } }) => ReactNode }) => (
        <>{children({ message: { role: 'assistant' } })}</>
      ),
    },
    MessagePrimitive: {
      Root: Wrap,
      Parts: () => <div className="agent-chat-message-text">CPU 占用偏高</div>,
      Error: ({ children }: { children?: ReactNode }) => <>{children}</>,
    },
    MessagePartPrimitive: { Text: Wrap },
    ComposerPrimitive: {
      Root: Wrap,
      Input: ({ submitMode: _submitMode, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement> & { submitMode?: string }) => <textarea {...props} />,
      Send: ({ children, ...props }: ButtonHTMLAttributes<HTMLButtonElement>) => <button type="button" {...props}>{children}</button>,
    },
  };
});

vi.mock('../styles/agentChat.css', () => ({}));

const noopSend = async () => '';
const noopChunk = () => () => {};

function renderPanel(override: Partial<ComponentProps<typeof AgentChatPanel>> = {}) {
  return render(
    <AgentChatPanel
      llm={emptyLLMConfig}
      collapsed={false}
      width={380}
      onToggle={vi.fn()}
      onWidthChange={vi.fn()}
      onSelectAgent={vi.fn()}
      onUpdateAgent={vi.fn()}
      agentChatSend={noopSend}
      onChunk={noopChunk}
      {...override}
    />,
  );
}

describe('AgentChatPanel', () => {
  it('展开时展示 assistant-ui 聊天区与输入框下方工具栏', () => {
    renderPanel();
    expect(screen.getByTestId('agent-panel')).toBeInTheDocument();
    expect(document.querySelector('.agent-panel-title')?.textContent).toBe('Codex');
    expect(screen.getByTestId('assistant-ui-chat')).toBeInTheDocument();
    const viewport = document.querySelector('.agent-chat-viewport');
    const footer = document.querySelector('.agent-chat-footer');
    expect(document.querySelector('.agent-chat-messages')).toBeInTheDocument();
    expect(viewport).toBeInTheDocument();
    expect(footer).toBeInTheDocument();
    expect(viewport).not.toContainElement(footer as HTMLElement);
    expect(screen.getByRole('status')).toHaveTextContent('Agent 正在分析性能数据');
    expect(screen.getByLabelText('询问性能问题…')).toBeInTheDocument();
    expect(screen.getByTestId('agent-composer-toolbar')).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: '切换 Agent' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: '选择模型' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: '选择 Effort' })).toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: /速度|Speed/i })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: '导出 Agent Chat 会话' })).toBeDisabled();
    expect(screen.getByRole('separator', { name: '调整 Agent 面板宽度' })).toBeInTheDocument();
  });

  it('完成一轮会话后可从顶部工具栏导出完整记录', async () => {
    const exportSession = vi.fn().mockResolvedValue(undefined);
    renderPanel({
      agentChatSend: vi.fn().mockResolvedValue('CPU 主线程耗时偏高。'),
      exportSession,
    });

    await act(async () => {
      const updates = assistantUiMock.adapter?.run({
        messages: [{ role: 'user', content: [{ type: 'text', text: '分析 frame 30' }] }],
      } as never) as AsyncIterable<unknown>;
      for await (const _update of updates) {
        // 消费完整流以触发会话记录完成状态。
      }
    });

    const button = screen.getByRole('button', { name: '导出 Agent Chat 会话' });
    expect(button).toBeEnabled();
    await userEvent.click(button);
    expect(exportSession).toHaveBeenCalledOnce();
    expect(exportSession.mock.calls[0][0]).toMatch(/^agent-chat-.*\.md$/);
    expect(exportSession.mock.calls[0][1]).toContain('分析 frame 30');
    expect(exportSession.mock.calls[0][1]).toContain('CPU 主线程耗时偏高。');
  });

  it('未配置 Agent 时提示去设置', () => {
    const empty = withActiveAgent(
      { ...emptyLLMConfig, agents: [{ id: 'x', name: '空', command: '', args: [], workDir: '', model: '', effort: 'medium' }] },
      'x',
    );
    renderPanel({ llm: empty });
    expect(screen.getByText(/请先在设置中配置 ACP Agent/)).toBeInTheDocument();
    expect(screen.queryByTestId('assistant-ui-chat')).not.toBeInTheDocument();
  });

  it('收起后面板变窄且可展开', async () => {
    const onToggle = vi.fn();
    renderPanel({ collapsed: true, onToggle });
    expect(screen.queryByRole('combobox', { name: '切换 Agent' })).not.toBeInTheDocument();
    expect(screen.queryByTestId('agent-resize')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: '展开 Agent 面板' }));
    expect(onToggle).toHaveBeenCalled();
  });

  it('切换 Agent、探测到的模型与 Effort 触发回调', async () => {
    const onSelectAgent = vi.fn();
    const onUpdateAgent = vi.fn();
    const listModels = vi.fn().mockResolvedValue([{ id: 'gpt-5.6-sol', label: 'GPT-5.6-Sol' }]);
    renderPanel({ onSelectAgent, onUpdateAgent, listModels });
    await waitFor(() => expect(screen.getByRole('option', { name: 'GPT-5.6-Sol' })).toBeInTheDocument());
    expect(listModels).toHaveBeenCalled();
    await userEvent.selectOptions(screen.getByRole('combobox', { name: '切换 Agent' }), 'cursor');
    expect(onSelectAgent).toHaveBeenCalledWith('cursor');
    await userEvent.selectOptions(screen.getByRole('combobox', { name: '选择模型' }), 'gpt-5.6-sol');
    expect(onUpdateAgent).toHaveBeenCalledWith({ id: 'codex', model: 'gpt-5.6-sol' });
    await userEvent.selectOptions(screen.getByRole('combobox', { name: '选择 Effort' }), 'high');
    expect(onUpdateAgent).toHaveBeenCalledWith({ id: 'codex', effort: 'high' });
  });

  it('Cursor 使用探测到的模型列表', async () => {
    const listModels = vi.fn().mockResolvedValue([{ id: 'composer-2.5', label: 'Composer 2.5' }]);
    renderPanel({ llm: withActiveAgent(emptyLLMConfig, 'cursor'), listModels });
    await waitFor(() => expect(screen.getByRole('option', { name: 'Composer 2.5' })).toBeInTheDocument());
    expect(listModels).toHaveBeenCalled();
    expect(screen.queryByRole('option', { name: 'Grok 4.6' })).not.toBeInTheDocument();
  });

  it('消息右键弹出复制菜单', () => {
    renderPanel();
    fireEvent.contextMenu(screen.getByText('CPU 占用偏高'), { clientX: 40, clientY: 80 });
    expect(screen.getByTestId('agent-chat-context-menu')).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '复制' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '复制消息' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '全选' })).toBeInTheDocument();
  });

  it('待插入引用在输入框中显示为可移除的行内标签，并向 Agent 写入原始引用', async () => {
    renderPanel({
      pendingReference: {
        id: 1,
        reference: { kind: 'module', type: 'memory', label: 'Memory', description: '优先调用 performance_memory_analysis。' },
      },
    });
    expect(assistantUiMock.setText).toHaveBeenCalledWith(expect.stringContaining('【性能引用|module|memory|Memory】'));
    expect(screen.getByText('Memory')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '移除 Memory 引用' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: '询问性能问题…' })).toHaveAttribute('contenteditable', 'true');
    await userEvent.click(screen.getByRole('button', { name: '移除 Memory 引用' }));
    expect(assistantUiMock.setText).toHaveBeenLastCalledWith('');
  });
});
