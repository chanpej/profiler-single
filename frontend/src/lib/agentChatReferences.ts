import type { FrameData, ProfilerModuleId } from '../types';
import { moduleById } from './profiler';

export type AgentChatReferenceKind = 'module' | 'frame';

export interface AgentChatReference {
  kind: AgentChatReferenceKind;
  type: string;
  label: string;
  description: string;
}

const MODULE_GUIDANCE: Record<ProfilerModuleId, string> = {
  cpu: '重点查看 CPU 总耗时、Main Thread、Scripts、GC 与 CPU Hierarchy；可调用 performance_module_analysis(module="cpu") 和 performance_cpu_hierarchy。',
  gpu: '重点查看 GPU 耗时与 Render Thread；可调用 performance_module_analysis(module="gpu")，并结合 performance_frame 核对单帧。',
  rendering: '重点查看 Draw Calls、Batches、SetPass、三角形和 Shadow Casters；可调用 performance_module_analysis(module="rendering")。',
  memory: '重点查看 Used/Committed Memory、GC、Gfx、纹理/网格/材质以及对象数量的趋势与峰值；优先调用 performance_memory_analysis，必要时调用 performance_module_analysis(module="memory")。',
  audio: '重点查看 Playing Sources、Audio Voices 与 Audio Memory；可调用 performance_module_analysis(module="audio")。',
  video: '重点查看 Video Memory；可调用 performance_module_analysis(module="video")。',
  physics: '重点查看 Active Rigidbodies、Colliders 与 Physics CPU；可调用 performance_module_analysis(module="physics")。',
  ui: '重点查看 UI Batches、UI Vertices 与 UI CPU；可调用 performance_module_analysis(module="ui")。',
};

/** 生成同时可被 Agent 理解、也可被聊天 UI 识别为内嵌卡片的模块引用。 */
export function createModuleReference(moduleId: ProfilerModuleId, selectedFrameIndex = -1): AgentChatReference {
  const module = moduleById(moduleId);
  const frameContext = selectedFrameIndex >= 0
    ? ` 当前选中帧的 MCP 索引为 ${selectedFrameIndex}，请优先围绕该帧核对模块数据。`
    : '';
  return {
    kind: 'module',
    type: moduleId,
    label: selectedFrameIndex >= 0 ? `${module.label} · Frame ${selectedFrameIndex + 1}` : module.label,
    description: `${MODULE_GUIDANCE[moduleId]}${frameContext}`,
  };
}

/** 当前帧索引是 MCP 使用的零基索引；label 保留 Player 记录的实际帧号。 */
export function createFrameReference(index: number, frame: FrameData | null): AgentChatReference {
  const displayIndex = frame?.frameIndex ?? index + 1;
  return {
    kind: 'frame', type: 'frame', label: `Frame ${displayIndex}`,
    description: `当前选中帧的 MCP 索引为 ${index}。请优先调用 performance_frame(index=${index}, includeHierarchy=true)；需要与其他帧对比时可调用 performance_frame_compare。`,
  };
}

/** 标记保持单行，确保原始消息仍然是可读、可导出的普通文本。 */
export function formatAgentChatReference(reference: AgentChatReference): string {
  return `【性能引用|${reference.kind}|${reference.type}|${reference.label}】${reference.description}【/性能引用】`;
}
