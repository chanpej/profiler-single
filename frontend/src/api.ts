// 对 Wails bindings 的薄封装：组件只依赖本模块，便于测试时 mock。
// 绑定层的 Go 切片可能为 null，这里统一归一化为空数组。
import { Events } from '@wailsio/runtime';
import * as PerfService from '../bindings/unityperfagent/services/perfservice.js';
import type { FrameData, Handshake, LLMConfig, Report, Stats, Status, TickEvent } from './types';

export const api = {
  connectUSB: (serial = '', packageId = ''): Promise<Handshake> => PerfService.ConnectUSB(serial, packageId),
  disconnect: (): Promise<void> => PerfService.Disconnect(),
  status: (): Promise<Status> => PerfService.Status(),
  frames: (offset: number, limit: number): Promise<FrameData[]> =>
    PerfService.Frames(offset, limit).then((rows) => rows ?? []),
  frameAt: (index: number): Promise<FrameData> => PerfService.FrameAt(index),
  latest: (n: number): Promise<FrameData[]> => PerfService.Latest(n).then((rows) => rows ?? []),
  stats: (): Promise<Stats> => PerfService.Stats(),
  analyze: (): Promise<Report> =>
    PerfService.Analyze().then((r) => ({ ...r, issues: r.issues ?? [] }) as Report),
  clear: (): Promise<void> => PerfService.Clear(),
  diagnose: (): Promise<string> => PerfService.Diagnose(),
  llmConfig: (): Promise<LLMConfig> =>
    PerfService.LLMConfig().then((cfg) => ({
      agentCommand: cfg.agentCommand ?? '',
      agentArgs: cfg.agentArgs ?? [],
      workDir: cfg.workDir ?? '',
    })),
  setLLMConfig: (cfg: LLMConfig): Promise<void> => PerfService.SetLLMConfig(cfg),
  onTick: (handler: (ev: TickEvent) => void): (() => void) =>
    Events.On('perf:tick', (ev: { data: TickEvent }) => handler(ev.data)),
};
