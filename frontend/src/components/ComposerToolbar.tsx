import { Gauge, Sparkles } from 'lucide-react';
import type { AgentProfile } from '../types';
import { EFFORT_OPTIONS, modelsForAgent, normalizeEffort, type ModelOption } from '../lib/agentRuntime';
import { AgentSwitcher } from './AgentSwitcher';

interface Props {
  agents: AgentProfile[];
  agent: AgentProfile;
  models?: ModelOption[];
  onSelectAgent: (id: string) => void;
  onUpdateAgent: (patch: Partial<AgentProfile> & { id: string }) => void;
  disabled?: boolean;
}

/** 输入框下方工具栏：Agent、模型、Effort。不提供速度/Fast 选择。 */
export function ComposerToolbar({
  agents, agent, models, onSelectAgent, onUpdateAgent, disabled = false,
}: Props) {
  const options = modelsForAgent(agent, models);
  const effort = normalizeEffort(agent.effort);

  return (
    <div className="agent-composer-tools" data-testid="agent-composer-toolbar">
      <AgentSwitcher
        agents={agents}
        activeId={agent.id}
        onChange={onSelectAgent}
        disabled={disabled}
      />
      <label className="agent-switcher">
        <Sparkles size={12} aria-hidden />
        <select
          className="agent-switcher-select agent-toolbar-model"
          aria-label="选择模型"
          title="选择模型"
          value={agent.model ?? ''}
          disabled={disabled}
          onChange={(e) => onUpdateAgent({ id: agent.id, model: e.target.value })}
        >
          <option value="">默认</option>
          {options.map((m) => (
            <option key={m.id} value={m.id}>{m.label}</option>
          ))}
        </select>
      </label>
      <label className="agent-switcher">
        <Gauge size={12} aria-hidden />
        <select
          className="agent-switcher-select agent-toolbar-effort"
          aria-label="选择 Effort"
          title="选择 Effort"
          value={effort}
          disabled={disabled}
          onChange={(e) => onUpdateAgent({ id: agent.id, effort: e.target.value })}
        >
          {EFFORT_OPTIONS.map((o) => (
            <option key={o.id} value={o.id}>{o.label}</option>
          ))}
        </select>
      </label>
    </div>
  );
}
