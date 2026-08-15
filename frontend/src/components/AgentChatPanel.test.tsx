import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ComponentProps } from 'react';
import { AgentChatPanel } from './AgentChatPanel';
import { emptyLLMConfig, withActiveAgent } from '../lib/llmConfig';

vi.mock('@nlux/react', () => ({
  AiChat: ({ events }: { events?: { messageSent?: () => void } }) => (
    <div data-testid="nlux-chat" className="nlux-AiChat-root">
      <div className="nlux-chatRoom-container nlux-chatRoom-starting">
        <div className="nlux-launchPad-container">
          <div className="nlux-comp-welcomeMessage">
            <div className="nlux-comp-avatar" data-testid="welcome-avatar" />
          </div>
        </div>
        <div className="nlux-conversation-container">
          <div className="nlux-comp-chatItem">
            <div className="nlux-comp-message">CPU 占用偏高</div>
          </div>
        </div>
        <div className="nlux-composer-container">
          <div className="nlux-comp-composer nlux-composer--typing" data-testid="nlux-composer">
            <textarea aria-label="询问性能问题…" defaultValue="待发送" />
            <button type="button" aria-label="发送" onClick={() => events?.messageSent?.()} />
          </div>
        </div>
      </div>
    </div>
  ),
  useAsStreamAdapter: (send: unknown) => send,
}));

vi.mock('@nlux/themes/nova.css', () => ({}));
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
  it('展开时展示标题、聊天区与输入框下方工具栏', () => {
    renderPanel();
    expect(screen.getByTestId('agent-panel')).toBeInTheDocument();
    expect(document.querySelector('.agent-panel-title')?.textContent).toBe('Codex');
    expect(screen.getByTestId('nlux-chat')).toBeInTheDocument();
    expect(screen.getByTestId('agent-composer-toolbar')).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: '切换 Agent' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: '选择模型' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: '选择 Effort' })).toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: /速度|Speed/i })).not.toBeInTheDocument();
    expect(screen.getByRole('separator', { name: '调整 Agent 面板宽度' })).toBeInTheDocument();
  });

  it('未配置 Agent 时提示去设置', () => {
    const empty = withActiveAgent(
      { ...emptyLLMConfig, agents: [{ id: 'x', name: '空', command: '', args: [], workDir: '', model: '', effort: 'medium' }] },
      'x',
    );
    renderPanel({ llm: empty });
    expect(screen.getByText(/请先在设置中配置 ACP Agent/)).toBeInTheDocument();
    expect(screen.queryByTestId('nlux-chat')).not.toBeInTheDocument();
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

  it('发送后清空输入框', async () => {
    renderPanel();
    const ta = screen.getByLabelText('询问性能问题…') as HTMLTextAreaElement;
    expect(ta.value).toBe('待发送');
    await userEvent.click(screen.getByRole('button', { name: '发送' }));
    expect(ta.value).toBe('');
  });

  it('进入停止态时清空输入框', async () => {
    renderPanel();
    const composer = screen.getByTestId('nlux-composer');
    composer.classList.remove('nlux-composer--typing');
    composer.classList.add('nlux-composer--waiting');
    await waitFor(() => {
      expect((screen.getByLabelText('询问性能问题…') as HTMLTextAreaElement).value).toBe('');
    });
  });

  it('消息右键弹出复制菜单', async () => {
    renderPanel();
    fireEvent.contextMenu(screen.getByText('CPU 占用偏高'), { clientX: 40, clientY: 80 });
    expect(screen.getByTestId('agent-chat-context-menu')).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '复制' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '复制消息' })).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: '全选' })).toBeInTheDocument();
  });
});
