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

export interface SampleNode {
  name: string;
  category: string;
  totalMs: number;
  selfMs: number;
  calls: number;
  gcAllocKB: number;
  /** 相对本帧起点的开始时间（ms），Timeline 用；缺省时按子节点顺序排布。 */
  startMs?: number;
  callstack?: string[];
  children?: SampleNode[];
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
  profilerStreamVersion?: number;
  deepProfilerSupported?: boolean;
  managedCallstacksSupported?: boolean;
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
  /** 为 true 时写入新帧；false 表示暂停抓取。 */
  recording?: boolean;
  /** 当前数据来自的存档路径；实时采集时为空。 */
  sourceFile?: string;
  deepProfiling?: boolean;
  callStacks?: boolean;
  deepProfilerSupported?: boolean;
  managedCallstacksSupported?: boolean;
  /** 最近一次采集异常中断的原因；正常断开、自动重连或未出错时为空。 */
  lastError?: string;
  /** 游戏把 Profiler 连接掐掉后，正在等回到前台自动重连。 */
  reconnecting?: boolean;
  /** 会话里是否有可导出的原始 Profiler 数据流。 */
  hasRaw?: boolean;
}

export interface AgentProfile {
  id: string;
  name: string;
  command: string;
  args: string[];
  workDir: string;
  model: string;
  effort: string;
}

export interface LLMConfig {
  activeId: string;
  agents: AgentProfile[];
  agentCommand: string;
  agentArgs: string[];
  workDir: string;
}

export interface ProbeResult {
  ok: boolean;
  name: string;
  message: string;
}

export interface TickEvent {
  totalFrames: number;
  retained: number;
  connected: boolean;
  recording?: boolean;
}

export interface ImportProgressEvent {
  phase: 'read' | 'parse' | string;
  done: number;
  total: number;
  percent: number;
  fileName: string;
}

export interface AgentModelOption {
  id: string;
  label: string;
}

export type OverlayId = 'frames' | 'analysis' | 'settings';

export type ProfilerModuleId =
  | 'cpu'
  | 'gpu'
  | 'rendering'
  | 'memory'
  | 'audio'
  | 'video'
  | 'physics'
  | 'ui';
