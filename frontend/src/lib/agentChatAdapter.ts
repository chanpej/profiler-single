export interface StreamObserver {
  next: (chunk: string) => void;
  error: (err: Error) => void;
  complete: () => void;
}

/** 把 ACP 聊天发送接到流式 observer（供 NLUX 等 UI 库使用）。 */
export function bindAgentStream(
  send: (message: string) => Promise<string>,
  onChunk: (handler: (text: string) => void) => () => void,
) {
  return (message: string, observer: StreamObserver) => {
    let streamed = '';
    const off = onChunk((text) => {
      streamed += text;
      observer.next(text);
    });
    Promise.resolve(send(message))
      .then((full) => {
        off();
        // Wails events and the RPC result use separate queues. A preliminary
        // chunk may arrive before the RPC completes while later chunks are
        // still queued, so append the part of the authoritative final result
        // that has not reached the UI yet.
        if (full.startsWith(streamed)) {
          const remaining = full.slice(streamed.length);
          if (remaining) observer.next(remaining);
        } else if (full && full !== streamed) {
          observer.next(full);
        }
        observer.complete();
      })
      .catch((e) => {
        off();
        observer.error(e instanceof Error ? e : new Error(String(e)));
      });
  };
}
