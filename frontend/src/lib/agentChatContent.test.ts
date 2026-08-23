import { describe, expect, it } from 'vitest';
import { parseAgentChatContent } from './agentChatContent';

describe('parseAgentChatContent', () => {
  it('splits intro, contiguous MCP progress, and answer into ordered segments', () => {
    expect(parseAgentChatContent(`开始分析。\n\n  【MCP 工具】读取性能数据  \n\n【MCP 工具】分析热点\n\n分析完成。`)).toEqual([
      { type: 'text', text: '开始分析。' },
      {
        type: 'tool',
        entries: ['【MCP 工具】读取性能数据', '【MCP 工具】分析热点'],
      },
      { type: 'text', text: '分析完成。' },
    ]);
  });

  it('keeps tool groups separate when normal text appears between them', () => {
    expect(parseAgentChatContent(`【MCP 工具】读取帧数据\n\n发现 CPU 峰值。\n继续检查。\n\n【MCP 工具】读取调用栈`)).toEqual([
      { type: 'tool', entries: ['【MCP 工具】读取帧数据'] },
      { type: 'text', text: '发现 CPU 峰值。\n继续检查。' },
      { type: 'tool', entries: ['【MCP 工具】读取调用栈'] },
    ]);
  });

  it('returns plain text as one text segment and trims only boundary blank lines', () => {
    expect(parseAgentChatContent(`\n\n第一段。\n\n第二段。\n\n`)).toEqual([
      { type: 'text', text: '第一段。\n\n第二段。' },
    ]);
  });

  it('recognizes generic tool progress lines', () => {
    expect(parseAgentChatContent('【工具】查询设备信息')).toEqual([
      { type: 'tool', entries: ['【工具】查询设备信息'] },
    ]);
  });

  it('recognizes the current streaming tool line without a trailing newline', () => {
    expect(parseAgentChatContent('正在分析\n【MCP 工具】读取当前帧')).toEqual([
      { type: 'text', text: '正在分析' },
      { type: 'tool', entries: ['【MCP 工具】读取当前帧'] },
    ]);
  });

  it('recognizes module and frame references as independent inline segments', () => {
    expect(parseAgentChatContent('请分析。\n【性能引用|module|memory|Memory】优先调用 performance_memory_analysis。\n【性能引用|frame|frame|Frame 42】MCP 索引为 7。')).toEqual([
      { type: 'text', text: '请分析。' },
      { type: 'reference', kind: 'module', referenceType: 'memory', label: 'Memory', description: '优先调用 performance_memory_analysis。' },
      { type: 'reference', kind: 'frame', referenceType: 'frame', label: 'Frame 42', description: 'MCP 索引为 7。' },
    ]);
  });

  it('recognizes a closed reference embedded between ordinary text', () => {
    expect(parseAgentChatContent('请检查【性能引用|module|memory|Memory · Frame 8】MCP 索引为 7。【/性能引用】的峰值。')).toEqual([
      { type: 'text', text: '请检查' },
      { type: 'reference', kind: 'module', referenceType: 'memory', label: 'Memory · Frame 8', description: 'MCP 索引为 7。' },
      { type: 'text', text: '的峰值。' },
    ]);
  });
});
