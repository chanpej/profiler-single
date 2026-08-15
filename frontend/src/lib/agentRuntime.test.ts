import { describe, expect, it } from 'vitest';
import {
  cursorModelSpec,
  isCodexAgent,
  isCursorAgent,
  modelsForAgent,
  normalizeEffort,
} from './agentRuntime';

describe('agentRuntime', () => {
  it('Effort 非法值回落到 medium', () => {
    expect(normalizeEffort('high')).toBe('high');
    expect(normalizeEffort('')).toBe('medium');
    expect(normalizeEffort('fast')).toBe('medium');
  });

  it('Cursor 模型规格始终禁用 fast', () => {
    expect(cursorModelSpec('composer-2.5', 'high')).toBe('composer-2.5[effort=high,fast=false]');
    expect(cursorModelSpec('grok-4.6', '')).toContain('fast=false');
    expect(cursorModelSpec('grok-4.6', 'high')).not.toContain('fast=true');
    expect(cursorModelSpec('', 'high')).toBe('');
  });

  it('Codex 与 Cursor 都使用探测列表，不硬编码模型', () => {
    expect(isCodexAgent({ id: 'codex', command: 'codex' })).toBe(true);
    expect(isCursorAgent({ id: 'cursor', command: 'agent' })).toBe(true);
    expect(modelsForAgent({ id: 'codex', command: 'codex', model: '' })).toEqual([]);
    expect(modelsForAgent({ id: 'cursor', command: 'agent', model: '' })).toEqual([]);
    const probed = modelsForAgent(
      { id: 'cursor', command: 'agent', model: '' },
      [{ id: 'composer-2.5', label: 'Composer 2.5' }],
    );
    expect(probed.map((m) => m.id)).toEqual(['composer-2.5']);
    expect(modelsForAgent({ id: 'codex', command: 'codex', model: 'kept' }, probed)[0].id).toBe('kept');
    expect(modelsForAgent({ id: 'x', command: 'agent', model: 'my-model' })[0].id).toBe('my-model');
  });
});
