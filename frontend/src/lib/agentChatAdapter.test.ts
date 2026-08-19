import { describe, expect, it, vi } from 'vitest';
import type { ChatModelAdapter } from '@assistant-ui/react';
import { createAgentChatAdapter } from './agentChatAdapter';

async function responseTexts(adapter: ChatModelAdapter, messages: unknown = [{ role: 'user', content: [{ type: 'text', text: '分析这帧' }] }]): Promise<string[]> {
  const result = adapter.run({ messages } as never) as AsyncIterable<{ content?: ReadonlyArray<{ type: string; text?: string }> }>;
  const updates: string[] = [];
  for await (const update of result) {
    updates.push(update.content?.[0]?.text ?? '');
  }
  return updates;
}

describe('createAgentChatAdapter', () => {
  it('将同一刷新周期内的密集片段合并为一次累计更新', async () => {
    const handlers: Array<(text: string) => void> = [];
    const send = vi.fn().mockImplementation(async () => {
      handlers.forEach((handler) => handler('你'));
      handlers.forEach((handler) => handler('好'));
      return '你好';
    });
    const onChunk = (handler: (text: string) => void) => {
      handlers.push(handler);
      return () => {};
    };

    await expect(responseTexts(createAgentChatAdapter({ send, onChunk }))).resolves.toEqual(['你好']);
    expect(send).toHaveBeenCalledWith('分析这帧');
  });

  it('分批到达的片段仍保持流式累计显示', async () => {
    const handlers: Array<(text: string) => void> = [];
    let finish!: (text: string) => void;
    const send = vi.fn(() => new Promise<string>((resolve) => { finish = resolve; }));
    const onChunk = (handler: (text: string) => void) => {
      handlers.push(handler);
      return () => {};
    };

    const result = responseTexts(createAgentChatAdapter({ send, onChunk, flushDelayMs: 5 }));
    await vi.waitFor(() => expect(handlers).toHaveLength(1));
    handlers[0]('你');
    await new Promise((resolve) => setTimeout(resolve, 10));
    handlers[0]('好');
    await new Promise((resolve) => setTimeout(resolve, 10));
    finish('你好');

    await expect(result).resolves.toEqual(['你', '你好']);
  });

  it('记录可导出的用户请求与最终回复', async () => {
    const onTurnUpdate = vi.fn();
    const send = vi.fn().mockResolvedValue('CPU 主线程耗时偏高');
    const onChunk = () => () => {};

    await responseTexts(createAgentChatAdapter({ send, onChunk, onTurnUpdate }));

    expect(onTurnUpdate).toHaveBeenCalledTimes(2);
    expect(onTurnUpdate.mock.calls[0][0]).toMatchObject({
      id: 1,
      user: '分析这帧',
      assistant: '',
      status: 'running',
    });
    expect(onTurnUpdate.mock.calls[1][0]).toMatchObject({
      id: 1,
      user: '分析这帧',
      assistant: 'CPU 主线程耗时偏高',
      status: 'complete',
    });
  });

  it('连续提问时只发送最新的用户问题', async () => {
    const send = vi.fn().mockResolvedValue('frame 30 分析结果');
    const onChunk = () => () => {};
    const messages = [
      { role: 'user', content: [{ type: 'text', text: '分析整个性能数据' }] },
      { role: 'assistant', content: [{ type: 'text', text: '整体分析结果' }] },
      { role: 'user', content: [{ type: 'text', text: 'frame 30 的耗时太高，分析其原因' }] },
    ];

    await responseTexts(createAgentChatAdapter({ send, onChunk }), messages);
    expect(send).toHaveBeenCalledWith('frame 30 的耗时太高，分析其原因');
  });

  it('使用 RPC 最终结果补齐尚未到达的文本', async () => {
    const handlers: Array<(text: string) => void> = [];
    const send = vi.fn().mockImplementation(async () => {
      handlers.forEach((handler) => handler('正在读取性能数据。'));
      return '正在读取性能数据。\n\n最终优化建议';
    });
    const onChunk = (handler: (text: string) => void) => {
      handlers.push(handler);
      return () => {};
    };

    await expect(responseTexts(createAgentChatAdapter({ send, onChunk }))).resolves.toEqual([
      '正在读取性能数据。',
      '正在读取性能数据。\n\n最终优化建议',
    ]);
  });

  it('没有事件片段时仍显示完整回复并清理订阅', async () => {
    const off = vi.fn();
    const send = vi.fn().mockResolvedValue('整段回复');
    const onChunk = vi.fn(() => off);

    await expect(responseTexts(createAgentChatAdapter({ send, onChunk }))).resolves.toEqual(['整段回复']);
    expect(off).toHaveBeenCalledOnce();
  });

  it('发送失败时将错误交给 assistant-ui', async () => {
    const send = vi.fn().mockRejectedValue(new Error('agent down'));
    const onChunk = () => () => {};

    await expect(responseTexts(createAgentChatAdapter({ send, onChunk }))).rejects.toThrow('agent down');
  });
});
