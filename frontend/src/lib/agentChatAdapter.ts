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
    let gotChunk = false;
    const off = onChunk((text) => {
      gotChunk = true;
      observer.next(text);
    });
    Promise.resolve(send(message))
      .then((full) => {
        off();
        if (full && !gotChunk) observer.next(full);
        observer.complete();
      })
      .catch((e) => {
        off();
        observer.error(e instanceof Error ? e : new Error(String(e)));
      });
  };
}
