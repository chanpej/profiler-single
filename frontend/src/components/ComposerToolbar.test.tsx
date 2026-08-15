import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ComposerToolbar } from './ComposerToolbar';
import { DEFAULT_AGENTS } from '../lib/llmConfig';

describe('ComposerToolbar', () => {
  it('列出 Agent、探测到的模型与 Effort，且没有速度选择', async () => {
    const onSelectAgent = vi.fn();
    const onUpdateAgent = vi.fn();
    render(
      <ComposerToolbar
        agents={DEFAULT_AGENTS}
        agent={DEFAULT_AGENTS[0]}
        models={[{ id: 'gpt-5.6-sol', label: 'GPT-5.6-Sol' }]}
        onSelectAgent={onSelectAgent}
        onUpdateAgent={onUpdateAgent}
      />,
    );
    expect(screen.getByTestId('agent-composer-toolbar')).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: '切换 Agent' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: '选择模型' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: '选择 Effort' })).toHaveValue('medium');
    expect(screen.queryByRole('combobox', { name: /速度|Speed|fast/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('option', { name: /^fast$/i })).not.toBeInTheDocument();

    await userEvent.selectOptions(screen.getByRole('combobox', { name: '切换 Agent' }), 'cursor');
    expect(onSelectAgent).toHaveBeenCalledWith('cursor');
    await userEvent.selectOptions(screen.getByRole('combobox', { name: '选择模型' }), 'gpt-5.6-sol');
    expect(onUpdateAgent).toHaveBeenCalledWith({ id: 'codex', model: 'gpt-5.6-sol' });
    await userEvent.selectOptions(screen.getByRole('combobox', { name: '选择 Effort' }), 'high');
    expect(onUpdateAgent).toHaveBeenCalledWith({ id: 'codex', effort: 'high' });
  });
});
