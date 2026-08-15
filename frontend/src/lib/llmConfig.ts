import type { AgentProfile, LLMConfig } from '../types';
import { DEFAULT_EFFORT, normalizeEffort } from './agentRuntime';

/** 与 Go DefaultAgents 一致：官方 ACP 启动参数。 */
export const DEFAULT_AGENTS: AgentProfile[] = [
  {
    id: 'codex',
    name: 'Codex',
    command: 'codex',
    args: [],
    workDir: '',
    model: '',
    effort: DEFAULT_EFFORT,
  },
  {
    id: 'cursor',
    name: 'Cursor CLI',
    command: 'agent',
    args: ['acp'],
    workDir: '',
    model: '',
    effort: DEFAULT_EFFORT,
  },
];

function normalizeAgent(a: Partial<AgentProfile> & Pick<AgentProfile, 'id'>): AgentProfile {
  return {
    id: a.id,
    name: a.name || a.id,
    command: a.command ?? '',
    args: a.args ?? [],
    workDir: a.workDir ?? '',
    model: (a.model ?? '').trim(),
    effort: normalizeEffort(a.effort),
  };
}

export const emptyLLMConfig: LLMConfig = {
  activeId: 'codex',
  agents: DEFAULT_AGENTS.map((a) => ({ ...a, args: [...a.args] })),
  agentCommand: DEFAULT_AGENTS[0].command,
  agentArgs: [...DEFAULT_AGENTS[0].args],
  workDir: '',
};

export function normalizeLLMConfig(raw: Partial<LLMConfig> | null | undefined): LLMConfig {
  const agents = (raw?.agents ?? []).filter((a) => a && a.id);
  const list = agents.length > 0
    ? agents.map((a) => normalizeAgent(a))
    : DEFAULT_AGENTS.map((a) => ({ ...a, args: [...a.args] }));
  let activeId = raw?.activeId ?? '';
  if (!list.some((a) => a.id === activeId)) activeId = list[0].id;
  const active = list.find((a) => a.id === activeId) ?? list[0];
  return {
    activeId,
    agents: list,
    agentCommand: active.command,
    agentArgs: active.args,
    workDir: active.workDir,
  };
}

export function activeAgent(cfg: LLMConfig): AgentProfile {
  return cfg.agents.find((a) => a.id === cfg.activeId) ?? cfg.agents[0] ?? DEFAULT_AGENTS[0];
}

export function withActiveAgent(cfg: LLMConfig, id: string): LLMConfig {
  return normalizeLLMConfig({ ...cfg, activeId: id });
}

export function isAgentConfigured(cfg: LLMConfig): boolean {
  return (activeAgent(cfg).command ?? '').trim() !== '';
}

export function argsToText(args: string[] | null | undefined): string {
  return (args ?? []).join(' ');
}

export function textToArgs(text: string): string[] {
  const t = text.trim();
  return t === '' ? [] : t.split(/\s+/);
}

export function updateAgent(cfg: LLMConfig, patch: Partial<AgentProfile> & { id: string }): LLMConfig {
  const agents = cfg.agents.map((a) => (a.id === patch.id ? { ...a, ...patch } : a));
  return normalizeLLMConfig({ ...cfg, agents, activeId: cfg.activeId });
}

export function addCustomAgent(cfg: LLMConfig): LLMConfig {
  let n = 1;
  let id = 'custom';
  while (cfg.agents.some((a) => a.id === id)) {
    n += 1;
    id = `custom-${n}`;
  }
  const agents = [...cfg.agents, { id, name: n === 1 ? '自定义' : `自定义 ${n}`, command: '', args: [], workDir: '', model: '', effort: DEFAULT_EFFORT }];
  return normalizeLLMConfig({ ...cfg, agents, activeId: id });
}
