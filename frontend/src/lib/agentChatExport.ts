import { Dialogs } from '@wailsio/runtime';
import type { AgentChatTurnRecord } from './agentChatAdapter';
import { isUserCancelled } from './userCancel';

const MARKDOWN_FILTERS = [{ DisplayName: 'Markdown (*.md)', Pattern: '*.md' }];

export function formatAgentChatSession(
  agentName: string,
  turns: readonly AgentChatTurnRecord[],
  exportedAt = new Date(),
): string {
  const lines = [
    '# Agent Chat Session',
    '',
    `- Agent: ${agentName || 'Agent'}`,
    `- Exported: ${exportedAt.toISOString()}`,
    `- Turns: ${turns.length}`,
  ];

  for (const [index, turn] of turns.entries()) {
    lines.push(
      '',
      `## Turn ${index + 1}`,
      '',
      `- Started: ${turn.startedAt}`,
      `- Status: ${turn.status}`,
    );
    if (turn.completedAt) lines.push(`- Completed: ${turn.completedAt}`);
    lines.push('', '### User', '', turn.user, '', '### Agent', '', turn.assistant || '_No response recorded_');
  }

  return `${lines.join('\n')}\n`;
}

export function agentChatExportFilename(now = new Date()): string {
  return `agent-chat-${now.toISOString().replace(/[:.]/g, '-')}.md`;
}

export async function chooseAgentChatExportPath(filename: string): Promise<string> {
  try {
    return await Dialogs.SaveFile({
      Title: '导出 Agent Chat 会话',
      Filename: filename,
      Filters: MARKDOWN_FILTERS,
      CanCreateDirectories: true,
    });
  } catch (error) {
    if (isUserCancelled(error)) return '';
    throw error;
  }
}
