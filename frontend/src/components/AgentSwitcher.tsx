import { Bot } from 'lucide-react';
import type { AgentProfile } from '../types';

interface Props {
  agents: AgentProfile[];
  activeId: string;
  onChange: (id: string) => void;
  disabled?: boolean;
}

/** 聊天输入区工具栏中的 Agent 切换列表。 */
export function AgentSwitcher({ agents, activeId, onChange, disabled = false }: Props) {
  return (
    <label className="agent-switcher">
      <Bot size={12} aria-hidden />
      <select
        className="agent-switcher-select"
        aria-label="切换 Agent"
        value={activeId}
        disabled={disabled || agents.length === 0}
        onChange={(e) => onChange(e.target.value)}
      >
        {agents.map((a) => (
          <option key={a.id} value={a.id}>{a.name}</option>
        ))}
      </select>
    </label>
  );
}
