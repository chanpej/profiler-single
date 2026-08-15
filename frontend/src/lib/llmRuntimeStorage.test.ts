import { afterEach, describe, expect, it } from 'vitest';
import { emptyLLMConfig, updateAgent, withActiveAgent } from './llmConfig';
import {
  applyLLMRuntime,
  captureLLMRuntime,
  loadLLMRuntime,
  saveLLMRuntime,
} from './llmRuntimeStorage';

describe('llmRuntimeStorage', () => {
  afterEach(() => localStorage.clear());

  it('按 Agent 缓存模型与 Effort，并还原上次选中的 Agent', () => {
    let cfg = withActiveAgent(emptyLLMConfig, 'codex');
    cfg = updateAgent(cfg, { id: 'codex', model: 'gpt-5.6-sol', effort: 'high' });
    cfg = updateAgent(cfg, { id: 'cursor', model: 'composer-2.5', effort: 'low' });
    cfg = withActiveAgent(cfg, 'cursor');
    saveLLMRuntime(cfg);

    const cache = loadLLMRuntime();
    expect(cache?.activeId).toBe('cursor');
    expect(cache?.agents.codex).toEqual({ model: 'gpt-5.6-sol', effort: 'high' });
    expect(cache?.agents.cursor).toEqual({ model: 'composer-2.5', effort: 'low' });

    const restored = applyLLMRuntime(emptyLLMConfig, cache);
    expect(restored.activeId).toBe('cursor');
    expect(restored.agents.find((a) => a.id === 'codex')).toMatchObject({ model: 'gpt-5.6-sol', effort: 'high' });
    expect(restored.agents.find((a) => a.id === 'cursor')).toMatchObject({ model: 'composer-2.5', effort: 'low' });
  });

  it('非法缓存忽略，Effort 非法值回落 medium', () => {
    expect(applyLLMRuntime(emptyLLMConfig, null).activeId).toBe('codex');
    localStorage.setItem('upa.llmRuntime', '{not json');
    expect(loadLLMRuntime()).toBeNull();

    const snap = captureLLMRuntime(updateAgent(emptyLLMConfig, { id: 'codex', model: 'x', effort: 'fast' }));
    expect(snap.agents.codex.effort).toBe('medium');
  });
});
