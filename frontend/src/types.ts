// 与 Go 后端结构对应的前端类型（结构化兼容 bindings 生成的类）。

export interface FrameData {
  frameIndex: number;
  timestampMs: number;
  fps: number;
  cpuTimeMs: number;
  gpuTimeMs: number;
  mainThreadMs: number;
  renderMs: number;
  gcAllocKB: number;
  usedMemMB: number;
  reservedMB: number;
  drawCalls: number;
  setPassCalls: number;
  batches: number;
  triangles: number;
  vertices: number;
  cpuRenderingMs: number;
  cpuScriptsMs: number;
  cpuPhysicsMs: number;
  cpuAnimationMs: number;
  cpuGcMs: number;
  cpuVsyncMs: number;
  cpuUiMs: number;
  cpuOthersMs: number;
  gcUsedMB: number;
  gcReservedMB: number;
  gfxUsedMB: number;
  gfxReservedMB: number;
  audioUsedMB: number;
  videoUsedMB: number;
  profilerUsedMB: number;
  profilerReservedMB: number;
  systemUsedMB: number;
  shadowCasters: number;
  playingSources: number;
  audioVoices: number;
  activeRigidbodies: number;
  colliders: number;
  uiBatches: number;
  uiVertices: number;
}

export interface Stats {
  frameCount: number;
  avgFps: number;
  minFps: number;
  maxFps: number;
  avgCpuMs: number;
  p95CpuMs: number;
  maxCpuMs: number;
  avgDrawCalls: number;
  maxDrawCalls: number;
  totalGcKb: number;
  lastUsedMb: number;
}

export type Severity = 'info' | 'warning' | 'critical';

export interface Issue {
  rule: string;
  severity: Severity;
  title: string;
  detail: string;
  frameIndex?: number;
}

export interface Report {
  frameCount: number;
  issues: Issue[];
  summary: string;
}

export interface Handshake {
  deviceName: string;
  deviceModel: string;
  os: string;
  unityVersion: string;
  appVersion: string;
}

export interface PlayerInfo {
  ip: string;
  port: number;
  flags: number;
  guid: string;
  id: string;
  debug: boolean;
  packageName: string;
  projectName: string;
  /** "adb" USB 端口转发。 */
  transport?: string;
  serial?: string;
}

export interface Status {
  connected: boolean;
  /** 采集通道：固定为 "profiler"（Unity Editor Profiler 协议）。 */
  mode: string;
  /** "adb" 表示 USB adb forward。 */
  transport?: string;
  device: Handshake;
  totalFrames: number;
  retained: number;
  /** 检测到的 Unity 版本是否为受支持的 2022.x。 */
  unitySupported: boolean;
}

export interface LLMConfig {
  agentCommand: string;
  agentArgs: string[];
  workDir: string;
}

export interface TickEvent {
  totalFrames: number;
  retained: number;
  connected: boolean;
}

export type ViewId = 'dashboard' | 'frames' | 'analysis' | 'settings';

export type ProfilerModuleId =
  | 'cpu'
  | 'gpu'
  | 'rendering'
  | 'memory'
  | 'audio'
  | 'video'
  | 'physics'
  | 'ui';
