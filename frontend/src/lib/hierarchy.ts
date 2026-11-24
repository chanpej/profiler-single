import type { SampleNode } from '../types';

export interface FlatSample {
  node: SampleNode;
  depth: number;
  path: string;
  hasChildren: boolean;
}

/** 用采样名拼路径，跨帧刷新后仍能对应同一行的展开状态。 */
export function samplePath(prefix: string, name: string): string {
  return prefix ? `${prefix}/${name}` : name;
}

/** 按展开集合把采样树展平，供 Hierarchy 表格渲染。 */
export function flattenVisible(nodes: SampleNode[], expanded: Set<string>, prefix = '', depth = 0): FlatSample[] {
  const out: FlatSample[] = [];
  nodes.forEach((n) => {
    const path = samplePath(prefix, n.name);
    const hasChildren = (n.children?.length ?? 0) > 0;
    out.push({ node: n, depth, path, hasChildren });
    if (hasChildren && expanded.has(path)) {
      out.push(...flattenVisible(n.children ?? [], expanded, path, depth + 1));
    }
  });
  return out;
}

/** 默认展开前 maxDepth 层，对齐 Unity Hierarchy 初始展开根节点。 */
export function defaultExpanded(nodes: SampleNode[], maxDepth = 2): Set<string> {
  const set = new Set<string>();
  const walk = (list: SampleNode[], prefix: string, depth: number) => {
    list.forEach((n) => {
      const path = samplePath(prefix, n.name);
      if (depth < maxDepth && (n.children?.length ?? 0) > 0) {
        set.add(path);
        walk(n.children ?? [], path, depth + 1);
      }
    });
  };
  walk(nodes, '', 0);
  return set;
}

/**
 * 通往调用栈采样的所有祖先路径。分配采样藏在 5~7 层深处，默认展开层数够不到，
 * 用户勾了 Call Stacks 也只会看到一棵没有调用栈的浅树。
 */
export function callstackPaths(nodes: SampleNode[], prefix = ''): Set<string> {
  const set = new Set<string>();
  const walk = (list: SampleNode[], pre: string): boolean => {
    let found = false;
    list.forEach((n) => {
      const path = samplePath(pre, n.name);
      const kids = n.children ?? [];
      const inKids = kids.length > 0 && walk(kids, path);
      if (inKids) set.add(path);
      if (inKids || (n.callstack?.length ?? 0) > 0) found = true;
    });
    return found;
  };
  walk(nodes, prefix);
  return set;
}

export interface ExpandOptions {
  maxDepth?: number;
  /** 额外展开通往调用栈的分支（勾选 Call Stacks 时用）。 */
  revealCallstacks?: boolean;
}

/**
 * 合并默认展开与用户操作：用户收起过的路径不会被数据刷新重新展开。
 */
export function mergeExpanded(
  current: Set<string>,
  nodes: SampleNode[],
  touched: Set<string>,
  opts: ExpandOptions = {},
): Set<string> {
  const { maxDepth = 2, revealCallstacks = false } = opts;
  const next = new Set(current);
  const auto = defaultExpanded(nodes, maxDepth);
  if (revealCallstacks) {
    for (const p of callstackPaths(nodes)) auto.add(p);
  }
  for (const p of auto) {
    if (!touched.has(p)) next.add(p);
  }
  return next;
}

const timelineColors = new Map<string, string>([
  ['Render', '#7B9E05'],
  ['VFX', '#7B9E05'],
  ['Scripts', '#3488A7'],
  ['Managed Jobs', '#3488A7'],
  ['Physics', '#CC7000'],
  ['Animation', '#5AB2BC'],
  ['GC', '#72680C'],
  ['GarbageCollector', '#72680C'],
  ['VSync', '#C5A206'],
  ['Lighting', '#882807'],
  ['UI Layout', '#614985'],
  ['UI Render', '#614985'],
  ['Other', '#7A7B1E'],
  ['Audio', '#50A05D'],
  ['Audio Job', '#7142B7'],
  ['Audio Update Job', '#A242B7'],
  ['Memory', '#B2194C'],
  ['Internal', '#646464'],
  ['Burst Jobs', '#2ECC71'],
  ['Build Interface', '#4EC9B0'],
  ['Input', '#4EC9F0'],
]);

export function categoryColor(category: string, sampleName?: string): string {
  if (sampleName === 'GC.Alloc') return timelineColors.get('Memory')!;
  return timelineColors.get(category) ?? timelineColors.get('Other')!;
}

/**
 * 按名字+分类合并同名兄弟，对齐 Unity Hierarchy 表。
 * Timeline 用未合并树；表格才合并，Calls / Total 才是累加值。
 */
export function mergeSampleNodes(nodes: SampleNode[]): SampleNode[] {
  if (nodes.length === 0) return [];
  const order: string[] = [];
  const byKey = new Map<string, { node: SampleNode; kids: SampleNode[] }>();
  for (const n of nodes) {
    const key = `${n.name}\0${n.category}`;
    let acc = byKey.get(key);
    if (!acc) {
      acc = { node: { ...n, children: undefined }, kids: [] };
      byKey.set(key, acc);
      order.push(key);
    } else {
      acc.node.totalMs += n.totalMs;
      acc.node.selfMs += n.selfMs;
      acc.node.calls += n.calls;
      acc.node.gcAllocKB += n.gcAllocKB;
      if ((n.startMs ?? Infinity) < (acc.node.startMs ?? Infinity)) {
        acc.node.startMs = n.startMs;
      }
      if ((!acc.node.callstack || acc.node.callstack.length === 0) && n.callstack?.length) {
        acc.node.callstack = n.callstack;
      }
    }
    if (n.children?.length) acc.kids.push(...n.children);
  }
  return order.map((key) => {
    const acc = byKey.get(key)!;
    const children = mergeSampleNodes(acc.kids);
    const node = children.length > 0 ? { ...acc.node, children } : acc.node;
    return node;
  }).sort((a, b) => b.totalMs - a.totalMs);
}
