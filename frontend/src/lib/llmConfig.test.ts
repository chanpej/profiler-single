import { describe, expect, it } from 'vitest';
import {
  addCustomAgent,
  DEFAULT_AGENTS,
  emptyLLMConfig,
  isAgentConfigured,
  normalizeLLMConfig,
  withActiveAgent,
} from './llmConfig';

describe('llmConfig', () => {
  it('默认包含 Codex 与 Cursor CLI 官方 ACP 参数', () => {
    expect(DEFAULT_AGENTS[0]).toMatchObject({
      id: 'codex',
      command: 'codex',
      args: [],
    });
    expect(DEFAULT_AGENTS[1]).toMatchObject({
      id: 'cursor',
      command: 'agent',
      args: ['acp'],
    });
    expect(isAgentConfigured(emptyLLMConfig)).toBe(true);
  });

  it('切换 active 同步 command', () => {
    const next = withActiveAgent(emptyLLMConfig, 'cursor');
    expect(next.activeId).toBe('cursor');
    expect(next.agentCommand).toBe('agent');
    expect(next.agentArgs).toEqual(['acp']);
  });

  it('空配置归一化为默认列表', () => {
    const got = normalizeLLMConfig({});
    expect(got.agents.map((a) => a.id)).toEqual(['codex', 'cursor']);
    expect(got.activeId).toBe('codex');
  });

  it('添加自定义 Agent', () => {
    const got = addCustomAgent(emptyLLMConfig);
    expect(got.activeId).toBe('custom');
    expect(got.agents.some((a) => a.id === 'custom')).toBe(true);
  });

  it('保留并归一化 model 与 effort', () => {
    const got = normalizeLLMConfig({
      activeId: 'codex',
      agents: [{ id: 'codex', name: 'Codex', command: 'codex', args: [], workDir: '', model: 'gpt-5.3-codex', effort: 'high' }],
    });
    expect(got.agents[0].model).toBe('gpt-5.3-codex');
    expect(got.agents[0].effort).toBe('high');
    const fallback = normalizeLLMConfig({
      agents: [{ id: 'codex', name: 'Codex', command: 'codex', args: [], workDir: '', model: '', effort: 'fast' }],
    });
    expect(fallback.agents[0].effort).toBe('medium');
  });
});
