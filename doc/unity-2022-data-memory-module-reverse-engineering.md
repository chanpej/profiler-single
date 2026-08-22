# Unity 2022 `.data` Memory Module 解析方法

本文记录 Unity 2022.3 Editor Profiler 离线 `.data` 文件中 Memory Module 的逆向分析方法。目标是让后续维护者能够从证据出发定位数据来源、扩展计数器，并验证解析结果是否与 Unity Editor 一致。

适用范围：Unity 2022.3 的 Editor Profiler `.data` 存档；当前验证样本为 `test/testdata/unity-profiler-proxy.data` 与同一次采集的 `test/same-session.uppc`。

## 问题背景

`.data` 文件前部包含 Legacy `MemoryStats` 和固定大小的 `AllProfilerStats`。最初解析器只从后者的已知偏移得到 `Audio Used Memory`，于是错误地认为 `.data` 没有其它 Memory 数据。

Unity Editor 实际可以在同一文件中显示 Total Used、GC、Gfx、Texture、Mesh、Material、AnimationClip、对象统计等完整 Simple 视图数据。因此必须以 Editor 的读取路径为准，而不能仅依据前部旧统计块推断文件能力。

## Editor 的读取路径

对安装目录中的 `UnityEditor.dll` 反编译 `UnityEditorInternal.Profiling.MemoryProfilerModule`，可以确认 Simple 视图的核心逻辑如下：

1. 取得选中帧的 `ProfilerDriver.GetRawFrameDataView(frame, 0)`。
2. 使用 `rawFrameDataView.GetMarkerId("Total Reserved Memory")` 判断该帧是否有 Memory counter 数据。
3. 对每个计数器以名称取得 marker ID。
4. 使用 `rawFrameDataView.GetCounterValueAsLong(markerId)` 取得原始字节数或数量。

Editor 使用的关键名称包括：

- `System Used Memory`、`App Committed Memory`、`Total Used Memory`、`Total Reserved Memory`
- `Gfx Reserved Memory`、`GC Used Memory`、`GC Reserved Memory`
- `Audio Used Memory`、`Video Used Memory`、`Profiler Used Memory`、`Profiler Reserved Memory`
- `Texture Count` / `Texture Memory`、`Mesh Count` / `Mesh Memory`
- `Material Count` / `Material Memory`
- `AnimationClip Count` / `AnimationClip Memory`
- `Asset Count`、`Game Object Count`、`Scene Object Count`、`Object Count`
- `GC Allocation In Frame Count`、`GC Allocated In Frame`

这也解释了一个重要现象：`GetMarkerId` 返回的是 marker 定义 ID（例如本样本的 `Total Used Memory` 为 1660），但 `.data` 帧尾 counter table 中保存的是会话内 counter-store ID，不可直接和 marker ID 等同。

## `.data` 中实际数据的位置与记录格式

完整的 Memory 值位于每帧体后半部的 raw counter table。单条标量记录的结构为：

```text
uint32 counterStoreID
uint32 valueCount        // 当前样本为 1
uint32 valueType         // 2: uint32，4: uint64
uint32 byteSize          // 4 或 8
byte   value[byteSize]
```

例如样本首个有效帧中，`Total Used Memory` 的记录值为 `408591878`，即 `389.7 MB`；它与 Editor 的 `GetCounterValueAsLong` 结果完全一致。

counter-store ID 会随帧变化，也会因未启用的内置计数器留下空洞，因此不能硬编码 ID，也不能要求 ID 严格连续。可靠的定位依据是：

1. Unity 2022.3 内置 Memory counter 的固定序列；
2. 每项的 value type / byte size 序列；
3. counter-store ID 整体严格递增；
4. 值之间的业务不变量，例如 `Total Used <= Total Reserved`、`GC Used <= GC Reserved`、对象计数和资源大小为合理非负值。

当前实现位于 `internal/capture/capture.go` 的 `findRawMemoryCounters`。它扫描帧体寻找完整的 34 项签名，命中后由 `applyRawMemoryCounters` 映射到统一的 `protocol.FrameData`。

## 映射与显示策略

`protocol.FrameData` 扩展了下列字段，以保留 Editor Simple 视图的完整信息：

- 分项内存：Material、AnimationClip、单帧 GC Allocated。
- 对象统计：AnimationClip Count、Asset Count、GC Allocation In Frame Count。
- `MemoryCounterMask`：标明每个值实际由采集源提供，避免将缺失项误显示为零。

协议帧从 v3 的 204 字节扩展到 v4 的 228 字节；解码仍兼容旧 68、164、204 字节帧。

前端的 Memory Simple 视图会显示 Material、Animation Clips、Assets 和 GC Allocated In Frame；Memory 折线图中的单帧 GC 曲线使用原始 Memory counter 值，而 CPU Hierarchy 的 `GCAllocKB` 仍保留采样树归因值，两者不可混用。

## 推荐分析流程

遇到新的 Unity 版本、计数器缺失或数值不一致时，按以下顺序排查：

1. 用目标 Unity Editor 打开 `.data`，确认具体帧和需要的 UI 数值。
2. 反编译对应版本 `UnityEditor.dll` 中的 Profiler Module，确认 Editor 使用的 counter 名称及读取 API。
3. 用临时 Editor 脚本调用 `ProfilerDriver.LoadProfile`、`GetRawFrameDataView`、`GetMarkerId`、`GetCounterValueAsLong`，导出指定帧的名称、marker ID 与值，作为真值表。
4. 在 `.data` 中搜索该原始数值的 little-endian 表示，查看周围字节，确认记录布局和相邻计数器顺序。
5. 不把 raw counter-store ID 误当作 marker ID；优先从序列、类型和数值关系定位。
6. 将结果接入统一协议，并同时更新前端可用性掩码、显示逻辑和旧协议兼容解码。
7. 用同会话 `.uppc` / `.upac` 与 `.data` 做逐帧比对，而不仅验证单帧截图。

## 验证基线

`internal/profilerproxy/TestProxyFixtureReplaysAgainstEditorData` 使用同会话样本进行验证：

- `.uppc` 比 `.data` 多 21 帧预热数据；从 `.uppc` 第 22 帧开始与 `.data` 第 1 帧对齐。
- 对齐后的 299 帧比较 CPU 时间、Memory 字节类值与对象计数。
- 当前基线要求最大 Memory 差值不超过 `0.000001 MB`，计数必须完全相等。

这项测试既验证二进制定位逻辑，也能在 Unity 修改序列、数据类型或持久化方式时尽早失败，而不是静默显示错误数据。

## 维护限制

- 本解析只承诺 Unity 2022.3；升级 Unity 时应重新执行上述真值导出与逐帧对照。
- Memory Module 的 `Detailed` 标签页是 Memory Profiler Package 的快照分析，和本文的帧计数器 Simple 视图不同；`.data` 的帧 counter 不等同于完整 native/managed heap snapshot。
- 当完整 counter table 不存在时，解析器保留 Legacy `AllProfilerStats` 中可确认的 Audio 值，并通过 `MemoryCounterMask` 表示其它项目不可用。
