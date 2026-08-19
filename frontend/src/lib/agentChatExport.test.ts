import { describe, expect, it } from 'vitest';
import { agentChatExportFilename, formatAgentChatSession } from './agentChatExport';

const exportedAt = new Date('2026-08-19T08:00:00.000Z');

describe('Agent Chat session export', () => {
  it('formats the complete ordered transcript as Markdown', () => {
    const markdown = formatAgentChatSession('Codex', [
      {
        id: 1,
        user: '分析 frame 30',
        assistant: '【MCP 工具】读取帧数据\n\nCPU 主线程耗时偏高。',
        status: 'complete',
        startedAt: '2026-08-19T07:58:00.000Z',
        completedAt: '2026-08-19T07:58:05.000Z',
      },
      {
        id: 2,
        user: '继续给出优化方案',
        assistant: '优先优化脚本热点。',
        status: 'complete',
        startedAt: '2026-08-19T07:59:00.000Z',
        completedAt: '2026-08-19T07:59:03.000Z',
      },
    ], exportedAt);

    expect(markdown).toContain('# Agent Chat Session');
    expect(markdown).toContain('- Agent: Codex');
    expect(markdown).toContain('- Turns: 2');
    expect(markdown.indexOf('分析 frame 30')).toBeLessThan(markdown.indexOf('继续给出优化方案'));
    expect(markdown).toContain('【MCP 工具】读取帧数据');
  });

  it('creates a filesystem-safe timestamped Markdown filename', () => {
    expect(agentChatExportFilename(exportedAt)).toBe('agent-chat-2026-08-19T08-00-00-000Z.md');
  });
});
