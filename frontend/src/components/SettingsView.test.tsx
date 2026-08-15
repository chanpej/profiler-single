import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SettingsView } from './SettingsView';
import { emptyLLMConfig } from '../lib/llmConfig';
import { loadChatFontSize } from '../lib/chatAppearance';

describe('SettingsView', () => {
  afterEach(() => {
    localStorage.clear();
    document.documentElement.style.removeProperty('--agent-chat-font-size');
  });
  it('左侧分组包含常规与 Agent', async () => {
    render(
      <SettingsView
        load={async () => emptyLLMConfig}
        save={vi.fn()}
        probe={vi.fn()}
      />,
    );
    await waitFor(() => expect(screen.getByTestId('settings-layout')).toBeInTheDocument());
    expect(screen.getByRole('button', { name: '常规' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Agent' })).toBeInTheDocument();
    expect(screen.getByTestId('settings-agent')).toBeInTheDocument();
    expect(screen.getByRole('listitem', { name: 'Codex' })).toBeInTheDocument();
    expect(screen.getByRole('listitem', { name: 'Cursor CLI' })).toBeInTheDocument();
  });

  it('切换到常规分组', async () => {
    render(
      <SettingsView
        load={async () => emptyLLMConfig}
        save={vi.fn()}
        probe={vi.fn()}
      />,
    );
    await userEvent.click(screen.getByRole('button', { name: '常规' }));
    expect(screen.getByTestId('settings-general')).toBeInTheDocument();
    expect(screen.queryByTestId('settings-agent')).not.toBeInTheDocument();
  });

  it('检测连接展示结果', async () => {
    const probe = vi.fn().mockResolvedValue({ ok: true, name: 'Codex', message: 'ACP 握手成功' });
    render(
      <SettingsView
        load={async () => emptyLLMConfig}
        save={vi.fn()}
        probe={probe}
      />,
    );
    await userEvent.click(screen.getByRole('button', { name: '检测 ACP 连接' }));
    await waitFor(() => expect(screen.getByTestId('probe-result')).toBeInTheDocument());
    expect(probe).toHaveBeenCalled();
    expect(screen.getByTestId('probe-result').textContent).toContain('握手成功');
  });

  it('检测中立即显示状态', async () => {
    let finish!: (v: { ok: boolean; name: string; message: string }) => void;
    const probe = vi.fn().mockReturnValue(new Promise((resolve) => {
      finish = resolve;
    }));
    render(
      <SettingsView
        load={async () => emptyLLMConfig}
        save={vi.fn()}
        probe={probe}
      />,
    );
    await userEvent.click(screen.getByRole('button', { name: '检测 ACP 连接' }));
    expect(screen.getByTestId('probe-result').textContent).toContain('正在检测');
    finish({ ok: false, name: 'Codex', message: '找不到可执行文件' });
    await waitFor(() => expect(screen.getByTestId('probe-result').textContent).toContain('找不到可执行文件'));
  });

  it('常规分组可调整聊天字号', async () => {
    render(
      <SettingsView
        load={async () => emptyLLMConfig}
        save={vi.fn()}
        probe={vi.fn()}
      />,
    );
    await userEvent.click(screen.getByRole('button', { name: '常规' }));
    const slider = screen.getByRole('slider', { name: 'Agent 聊天字号' });
    fireEvent.change(slider, { target: { value: '14' } });
    expect(loadChatFontSize()).toBe(14);
    expect(document.documentElement.style.getPropertyValue('--agent-chat-font-size')).toBe('14px');
  });

  it('保存当前 Agent 配置', async () => {
    const save = vi.fn().mockResolvedValue(undefined);
    render(
      <SettingsView
        load={async () => emptyLLMConfig}
        save={save}
        probe={vi.fn()}
      />,
    );
    await userEvent.click(screen.getByRole('button', { name: '保存配置' }));
    await waitFor(() => expect(save).toHaveBeenCalled());
    const cfg = save.mock.calls[0][0];
    expect(cfg.activeId).toBe('codex');
    expect(cfg.agentCommand).toBe('codex');
  });
});
