import type { AgentProfile } from '../types';

export const EFFORT_OPTIONS = [
  { id: 'low', label: '低' },
  { id: 'medium', label: '中' },
  { id: 'high', label: '高' },
  { id: 'xhigh', label: '极高' },
] as const;

export type AgentEffort = (typeof EFFORT_OPTIONS)[number]['id'];

export const DEFAULT_EFFORT: AgentEffort = 'medium';

export type ModelOption = { id: string; label: string };

function commandBase(command: string): string {
  const leaf = command.replace(/\\/g, '/').split('/').pop() ?? '';
  return leaf.toLowerCase().replace(/\.(exe|cmd|bat)$/i, '');
}

export function isCursorAgent(agent: Pick<AgentProfile, 'id' | 'command'>): boolean {
  const base = commandBase(agent.command);
  return agent.id === 'cursor' || base === 'agent' || base === 'cursor-agent';
}

export function isCodexAgent(agent: Pick<AgentProfile, 'id' | 'command'>): boolean {
  return agent.id === 'codex' || commandBase(agent.command) === 'codex';
}

export function normalizeEffort(raw: string | null | undefined): AgentEffort {
  const v = (raw ?? '').trim();
  return EFFORT_OPTIONS.some((o) => o.id === v) ? v as AgentEffort : DEFAULT_EFFORT;
}

/** Cursor `--model` 规格。始终 fast=false，不提供 Fast 档。 */
export function cursorModelSpec(model: string, effort: string): string {
  const m = model.trim();
  if (!m) return '';
  return `${m}[effort=${normalizeEffort(effort)},fast=false]`;
}

/** Codex / Cursor 均使用探测到的模型列表，不硬编码。 */
export function modelsForAgent(
  agent: Pick<AgentProfile, 'id' | 'command' | 'model'>,
  discovered: ModelOption[] = [],
): ModelOption[] {
  const list = discovered;
  const model = (agent.model ?? '').trim();
  if (model && !list.some((m) => m.id === model)) {
    return [{ id: model, label: model }, ...list];
  }
  return list;
}
