import { describe, expect, it } from 'vitest';
import { callstackPaths, categoryColor, defaultExpanded, flattenVisible, mergeExpanded, mergeSampleNodes } from './hierarchy';
import type { SampleNode } from '../types';

const tree: SampleNode[] = [
  {
    name: 'Main Thread',
    category: 'Internal',
    totalMs: 16,
    selfMs: 1,
    calls: 1,
    gcAllocKB: 0,
    children: [
      {
        name: 'BehaviourUpdate',
        category: 'Scripts',
        totalMs: 8,
        selfMs: 2,
        calls: 1,
        gcAllocKB: 0,
        children: [
          { name: 'PlayerController.Update', category: 'Scripts', totalMs: 6, selfMs: 6, calls: 1, gcAllocKB: 4, callstack: ['GC.Alloc', 'Update'] },
        ],
      },
    ],
  },
];

/** 把 leaf 套进一串单链父节点里，模拟真机上很深的调用层级。 */
function nest(names: string[], leaf: SampleNode): SampleNode {
  return names.reduceRight<SampleNode>((child, name) => ({
    name, category: 'Internal', totalMs: 1, selfMs: 0, calls: 1, gcAllocKB: 0, children: [child],
  }), leaf);
}

describe('hierarchy', () => {
  it('默认展开两层（按采样名路径）', () => {
    const exp = defaultExpanded(tree, 2);
    expect([...exp]).toContain('Main Thread');
    expect([...exp]).toContain('Main Thread/BehaviourUpdate');
  });

  it('按展开集合展平', () => {
    const exp = new Set(['Main Thread']);
    const rows = flattenVisible(tree, exp);
    expect(rows.map((r) => r.node.name)).toEqual(['Main Thread', 'BehaviourUpdate']);
    expect(rows[1].depth).toBe(1);
    expect(rows[1].path).toBe('Main Thread/BehaviourUpdate');
  });

  it('用户收起后合并默认展开不会重新打开', () => {
    const touched = new Set(['Main Thread']);
    const cur = new Set<string>();
    const got = mergeExpanded(cur, tree, touched);
    expect(got.has('Main Thread')).toBe(false);
    expect(got.has('Main Thread/BehaviourUpdate')).toBe(true);
  });

  // 真机上分配采样在 5~7 层深，默认展开两层够不到，勾了 Call Stacks 也看不到东西。
  it('勾选 Call Stacks 时展开到深处的调用栈采样', () => {
    const leaf: SampleNode = {
      name: 'GC.Alloc', category: 'GC', totalMs: 0, selfMs: 0, calls: 1, gcAllocKB: 2,
      callstack: ['0x1', 'UIPanel.FillDrawCall()'],
    };
    const deep: SampleNode[] = [nest(['Main Thread', 'PlayerLoop', 'Update', 'UIPanel.LateUpdate', 'FillDrawCall'], leaf)];

    const paths = callstackPaths(deep);
    expect(paths.has('Main Thread/PlayerLoop/Update/UIPanel.LateUpdate/FillDrawCall')).toBe(true);
    // 不含调用栈的分支不该被强行展开。
    expect(callstackPaths([nest(['A', 'B'], { ...leaf, callstack: [] })]).size).toBe(0);

    const off = flattenVisible(deep, mergeExpanded(new Set(), deep, new Set()));
    expect(off.some((r) => r.node.name === 'GC.Alloc')).toBe(false);

    const on = flattenVisible(deep, mergeExpanded(new Set(), deep, new Set(), { revealCallstacks: true }));
    expect(on.some((r) => r.node.name === 'GC.Alloc')).toBe(true);
  });

  it('用户收起的路径不会被调用栈展开覆盖', () => {
    const touched = new Set(['Main Thread']);
    const got = mergeExpanded(new Set(), tree, touched, { revealCallstacks: true });
    expect(got.has('Main Thread')).toBe(false);
  });

  it('分类颜色', () => {
    expect(categoryColor('Scripts')).toBe('#3B6CB5');
    expect(categoryColor('Unknown')).toBe('#5D6D7E');
  });

  it('按名字合并同名兄弟并累加 Calls / Total', () => {
    const got = mergeSampleNodes([
      { name: 'Update', category: 'Scripts', totalMs: 1, selfMs: 0.5, calls: 1, gcAllocKB: 0, startMs: 4 },
      { name: 'Update', category: 'Scripts', totalMs: 2, selfMs: 1, calls: 1, gcAllocKB: 1, startMs: 1, callstack: ['A'] },
      { name: 'LateUpdate', category: 'Scripts', totalMs: 0.4, selfMs: 0.4, calls: 1, gcAllocKB: 0, startMs: 8 },
    ]);
    expect(got).toHaveLength(2);
    expect(got[0].name).toBe('Update');
    expect(got[0].calls).toBe(2);
    expect(got[0].totalMs).toBe(3);
    expect(got[0].startMs).toBe(1);
    expect(got[0].callstack).toEqual(['A']);
    expect(got[1].name).toBe('LateUpdate');
  });
});
