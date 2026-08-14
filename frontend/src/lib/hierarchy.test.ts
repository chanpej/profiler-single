import { describe, expect, it } from 'vitest';
import { categoryColor, defaultExpanded, flattenVisible, mergeExpanded } from './hierarchy';
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

  it('分类颜色', () => {
    expect(categoryColor('Scripts')).toBe('#3B6CB5');
    expect(categoryColor('Unknown')).toBe('#5D6D7E');
  });
});
