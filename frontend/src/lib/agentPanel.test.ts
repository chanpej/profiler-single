import { afterEach, describe, expect, it } from 'vitest';
import {
  clampAgentWidth,
  COLLAPSED_AGENT_WIDTH,
  DEFAULT_AGENT_WIDTH,
  loadAgentCollapsed,
  loadAgentWidth,
  MAX_AGENT_WIDTH,
  MIN_AGENT_WIDTH,
  saveAgentCollapsed,
  saveAgentWidth,
  widthFromDrag,
} from './agentPanel';

describe('agentPanel', () => {
  afterEach(() => localStorage.clear());

  it('限制宽度范围', () => {
    expect(clampAgentWidth(10)).toBe(MIN_AGENT_WIDTH);
    expect(clampAgentWidth(9999)).toBe(MAX_AGENT_WIDTH);
    expect(clampAgentWidth(Number.NaN)).toBe(DEFAULT_AGENT_WIDTH);
    expect(COLLAPSED_AGENT_WIDTH).toBeLessThan(MIN_AGENT_WIDTH);
  });

  it('持久化宽度与收起状态', () => {
    expect(loadAgentWidth()).toBe(DEFAULT_AGENT_WIDTH);
    expect(loadAgentCollapsed()).toBe(false);
    saveAgentWidth(400);
    saveAgentCollapsed(true);
    expect(loadAgentWidth()).toBe(400);
    expect(loadAgentCollapsed()).toBe(true);
  });

  it('向左拖拽增加宽度', () => {
    expect(widthFromDrag(380, 1000, 900)).toBe(480);
    expect(widthFromDrag(380, 1000, 1100)).toBe(MIN_AGENT_WIDTH);
  });
});
