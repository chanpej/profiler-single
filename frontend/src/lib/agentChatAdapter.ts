import type { ChatModelAdapter, ThreadMessage } from '@assistant-ui/react';

export type AgentChatTurnRecord = {
  id: number;
  user: string;
  assistant: string;
  status: 'running' | 'complete' | 'error';
  startedAt: string;
  completedAt?: string;
};

export type AgentChatTransport = {
  send: (message: string) => Promise<string>;
  onChunk: (handler: (text: string) => void) => () => void;
  onTurnUpdate?: (turn: AgentChatTurnRecord) => void;
  /** 合并密集流式片段的刷新间隔；生产环境按约一帧刷新一次。 */
  flushDelayMs?: number;
};

const DEFAULT_STREAM_FLUSH_DELAY_MS = 24;

function latestUserText(messages: readonly ThreadMessage[]): string {
  const message = [...messages].reverse().find((item) => item.role === 'user');
  if (!message || message.role !== 'user') return '';
  return message.content
    .filter((part): part is Extract<typeof part, { type: 'text' }> => part.type === 'text')
    .map((part) => part.text)
    .join('');
}

/** 将 ACP 的 Wails 事件流转换为 assistant-ui 的累计消息更新。 */
export function createAgentChatAdapter({ send, onChunk, onTurnUpdate, flushDelayMs = DEFAULT_STREAM_FLUSH_DELAY_MS }: AgentChatTransport): ChatModelAdapter {
  let nextTurnId = 1;
  return {
    async *run({ messages }) {
      const message = latestUserText(messages);
      const turn: AgentChatTurnRecord = {
        id: nextTurnId++,
        user: message,
        assistant: '',
        status: 'running',
        startedAt: new Date().toISOString(),
      };
      onTurnUpdate?.(turn);
      let streamed = '';
      let completed = false;
      let finalText = '';
      let failure: Error | undefined;
      const chunks: string[] = [];
      let notify: (() => void) | undefined;
      let updatePending = false;
      let flushTimer: ReturnType<typeof setTimeout> | undefined;

      const signalUpdate = () => {
        if (notify) {
          const resolve = notify;
          notify = undefined;
          resolve();
        } else {
          updatePending = true;
        }
      };
      const flushNow = () => {
        if (flushTimer !== undefined) {
          clearTimeout(flushTimer);
          flushTimer = undefined;
        }
        signalUpdate();
      };
      const scheduleFlush = () => {
        if (flushTimer !== undefined) return;
        flushTimer = setTimeout(() => {
          flushTimer = undefined;
          signalUpdate();
        }, Math.max(0, flushDelayMs));
      };
      const waitForUpdate = () => {
        if (updatePending) {
          updatePending = false;
          return Promise.resolve();
        }
        return new Promise<void>((resolve) => {
          notify = resolve;
        });
      };
      const off = onChunk((text) => {
        if (!text) return;
        chunks.push(text);
        scheduleFlush();
      });

      void Promise.resolve(send(message)).then(
        (result) => {
          finalText = result;
          completed = true;
          flushNow();
        },
        (error) => {
          failure = error instanceof Error ? error : new Error(String(error));
          completed = true;
          flushNow();
        },
      );

      try {
        while (!completed || chunks.length > 0) {
          await waitForUpdate();
          if (chunks.length === 0) continue;
          streamed += chunks.splice(0).join('');
          yield { content: [{ type: 'text', text: streamed }] };
        }
        if (failure) {
          onTurnUpdate?.({
            ...turn,
            assistant: failure.message,
            status: 'error',
            completedAt: new Date().toISOString(),
          });
          throw failure;
        }
        // RPC 返回值是权威全文；Wails 事件和 RPC 走不同队列，不能假设顺序。
        if (finalText !== streamed) {
          yield { content: [{ type: 'text', text: finalText }] };
        }
        onTurnUpdate?.({
          ...turn,
          assistant: finalText || streamed,
          status: 'complete',
          completedAt: new Date().toISOString(),
        });
      } finally {
        if (flushTimer !== undefined) clearTimeout(flushTimer);
        notify = undefined;
        off();
      }
    },
  };
}
