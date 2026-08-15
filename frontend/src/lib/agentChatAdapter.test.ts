import { describe, expect, it, vi } from 'vitest';
import { bindAgentStream, type StreamObserver } from './agentChatAdapter';

function observer(): StreamObserver & { chunks: string[]; errors: Error[]; done: boolean } {
  const o = {
    chunks: [] as string[],
    errors: [] as Error[],
    done: false,
    next: (c: string) => { o.chunks.push(c); },
    error: (e: Error) => { o.errors.push(e); },
    complete: () => { o.done = true; },
  };
  return o;
}

describe('bindAgentStream', () => {
  it('转发流式片段并 complete', async () => {
    const handlers: Array<(t: string) => void> = [];
    const send = vi.fn().mockImplementation(async () => {
      handlers.forEach((h) => h('你'));
      handlers.forEach((h) => h('好'));
      return '你好';
    });
    const onChunk = (h: (t: string) => void) => {
      handlers.push(h);
      return () => {};
    };
    const obs = observer();
    bindAgentStream(send, onChunk)('hi', obs);
    await vi.waitFor(() => expect(obs.done).toBe(true));
    expect(obs.chunks.join('')).toBe('你好');
    expect(obs.errors).toHaveLength(0);
  });

  it('没有 chunk 时回退到完整文本', async () => {
    const send = vi.fn().mockResolvedValue('整段回复');
    const onChunk = () => () => {};
    const obs = observer();
    bindAgentStream(send, onChunk)('q', obs);
    await vi.waitFor(() => expect(obs.done).toBe(true));
    expect(obs.chunks).toEqual(['整段回复']);
  });

  it('发送失败时 error', async () => {
    const send = vi.fn().mockRejectedValue(new Error('agent down'));
    const onChunk = () => () => {};
    const obs = observer();
    bindAgentStream(send, onChunk)('q', obs);
    await vi.waitFor(() => expect(obs.errors.length).toBe(1));
    expect(obs.errors[0].message).toBe('agent down');
    expect(obs.done).toBe(false);
  });
});
