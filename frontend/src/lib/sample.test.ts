import { describe, expect, it } from 'vitest';
import { normalizeSampleTree } from './sample';

describe('normalizeSampleTree', () => {
  it('null 归一化为空数组', () => {
    expect(normalizeSampleTree(null)).toEqual([]);
    expect(normalizeSampleTree(undefined)).toEqual([]);
  });

  it('去掉 callstack / children 的 null', () => {
    const got = normalizeSampleTree([
      {
        name: 'Main Thread',
        category: 'Internal',
        totalMs: 16,
        selfMs: 1,
        calls: 1,
        gcAllocKB: 0,
        callstack: null,
        children: [
          {
            name: 'GC.Alloc',
            category: 'GC',
            totalMs: 0.1,
            selfMs: 0.1,
            calls: 1,
            gcAllocKB: 4,
            callstack: ['GC.Alloc'],
            children: null,
          },
        ],
      },
    ]);
    expect(got[0].callstack).toBeUndefined();
    expect(got[0].children?.[0].callstack).toEqual(['GC.Alloc']);
    expect(got[0].children?.[0].children).toBeUndefined();
    expect(got[0].startMs).toBeUndefined();
  });

  it('保留 startMs', () => {
    const got = normalizeSampleTree([{
      name: 'Main Thread',
      category: 'Internal',
      totalMs: 10,
      selfMs: 1,
      calls: 1,
      gcAllocKB: 0,
      startMs: 2.5,
    }]);
    expect(got[0].startMs).toBe(2.5);
  });
});
