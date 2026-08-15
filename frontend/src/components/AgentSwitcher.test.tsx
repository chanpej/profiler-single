import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AgentSwitcher } from './AgentSwitcher';
import { DEFAULT_AGENTS } from '../lib/llmConfig';

describe('AgentSwitcher', () => {
  it('列出 Agent 并切换', async () => {
    const onChange = vi.fn();
    render(
      <AgentSwitcher
        agents={DEFAULT_AGENTS}
        activeId="codex"
        onChange={onChange}
      />,
    );
    const sel = screen.getByRole('combobox', { name: '切换 Agent' });
    expect(sel).toHaveValue('codex');
    await userEvent.selectOptions(sel, 'cursor');
    expect(onChange).toHaveBeenCalledWith('cursor');
  });
});
