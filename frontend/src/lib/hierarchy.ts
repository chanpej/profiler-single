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

export function categoryColor(category: string): string {
  switch (category) {
    case 'Render': return '#2E8B4A';
    case 'Scripts': return '#3B6CB5';
    case 'Physics':
    case 'Physics2D': return '#8E44AD';
    case 'Animation': return '#D35400';
    case 'GC': return '#A0522D';
    case 'VSync': return '#7F8C8D';
    case 'Gui':
    case 'UI Layout':
    case 'UI Render':
    case 'UI Details': return '#C9A227';
    default: return '#5D6D7E';
  }
}
