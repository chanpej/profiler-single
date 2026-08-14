import '@testing-library/jest-dom/vitest';

// jsdom 未实现 ResizeObserver。@tanstack/react-virtual 依赖它测量视口，
// 这里在 observe 时立即回调一个固定尺寸（与下方 getBoundingClientRect 一致）。
class ResizeObserverStub {
  private cb: ResizeObserverCallback;
  constructor(cb: ResizeObserverCallback) {
    this.cb = cb;
  }
  observe(target: Element) {
    this.cb(
      [
        {
          target,
          contentRect: target.getBoundingClientRect(),
          borderBoxSize: [{ inlineSize: 1024, blockSize: 600 }],
          contentBoxSize: [{ inlineSize: 1024, blockSize: 600 }],
          devicePixelContentBoxSize: [{ inlineSize: 1024, blockSize: 600 }],
        } as unknown as ResizeObserverEntry,
      ],
      this as unknown as ResizeObserver,
    );
  }
  unobserve() {}
  disconnect() {}
}

globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;

// jsdom 的 canvas 没有 2D 上下文实现，返回 null 即可（组件已判空）。
HTMLCanvasElement.prototype.getContext = (() => null) as unknown as typeof HTMLCanvasElement.prototype.getContext;

// jsdom 不做布局，所有元素尺寸为 0，虚拟化列表将渲染不出任何行。
// 统一给出一个固定视口尺寸，让 @tanstack/react-virtual 能计算可视区域。
Element.prototype.getBoundingClientRect = function () {
  return {
    width: 1024,
    height: 600,
    top: 0,
    left: 0,
    bottom: 600,
    right: 1024,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect;
};
