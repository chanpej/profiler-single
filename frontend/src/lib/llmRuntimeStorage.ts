import type { LLMConfig } from '../types';
import { normalizeEffort } from './agentRuntime';
import { updateAgent, withActiveAgent } from './llmConfig';

const STORAGE_KEY = 'upa.llmRuntime';

export interface AgentRuntimeSelection {
  model: string;
  effort: string;
}

/** 上次选中的 Agent，以及每个 Agent 各自的模型 / Effort。 */
export interface LLMRuntimeCache {
  activeId: string;
  agents: Record<string, AgentRuntimeSelection>;
}

export function captureLLMRuntime(cfg: LLMConfig): LLMRuntimeCache {
  const agents: Record<string, AgentRuntimeSelection> = {};
  for (const a of cfg.agents) {
    agents[a.id] = {
      model: (a.model ?? '').trim(),
      effort: normalizeEffort(a.effort),
    };
  }
  return { activeId: cfg.activeId, agents };
}

export function applyLLMRuntime(cfg: LLMConfig, cache: LLMRuntimeCache | null | undefined): LLMConfig {
  if (!cache) return cfg;
  let next = cfg;
  if (cache.agents) {
    for (const a of cfg.agents) {
      const sel = cache.agents[a.id];
      if (!sel) continue;
      next = updateAgent(next, {
        id: a.id,
        model: (sel.model ?? '').trim(),
        effort: normalizeEffort(sel.effort),
      });
    }
  }
  const activeId = (cache.activeId ?? '').trim();
  if (activeId && next.agents.some((a) => a.id === activeId)) {
    next = withActiveAgent(next, activeId);
  }
  return next;
}

export function loadLLMRuntime(): LLMRuntimeCache | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as LLMRuntimeCache;
    if (!parsed || typeof parsed !== 'object') return null;
    const agents: Record<string, AgentRuntimeSelection> = {};
    if (parsed.agents && typeof parsed.agents === 'object') {
      for (const [id, sel] of Object.entries(parsed.agents)) {
        if (!id || !sel || typeof sel !== 'object') continue;
        agents[id] = {
          model: typeof sel.model === 'string' ? sel.model : '',
          effort: normalizeEffort(typeof sel.effort === 'string' ? sel.effort : ''),
        };
      }
    }
    return {
      activeId: typeof parsed.activeId === 'string' ? parsed.activeId : '',
      agents,
    };
  } catch {
    return null;
  }
}

export function saveLLMRuntime(cfg: LLMConfig): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(captureLLMRuntime(cfg)));
  } catch {
    // WebView 存储不可用时忽略。
  }
}
