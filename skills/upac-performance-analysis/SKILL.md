---
name: upac-performance-analysis
description: 从 unityperf 的 Unity 2022 .upac 录制精确提取帧、CPU 调用树、线程时间线、调用次数、内存计数器及截图时间偏差，并结合对应版本的 Unity 游戏源码定位卡顿、GC 和性能热点。用于 UPAC 离线分析、源码归因和优化前后复测。
---

# UPAC 性能分析

## 准备输入

先在 Skill 文件夹运行 `.\tools\upac.exe -file 'D:\captures\battle.upac' -metadata`，读取录制时的设备信息。`phone` 是匹配该 Player 的 ADB 手机快照，包含品牌、型号、Android/API、ABI、屏幕尺寸、总内存、ADB 标识、Wi-Fi IP 与端口。`handshake` 提供 Player 上报的 Unity 和应用版本。报告注明来源，比较两份录制时核对机型、系统和分辨率。老存档或采集时缺少可匹配设备时 `phone` 为 `null`，使用握手中的型号与系统，其余缺失项标记为未知。屏幕尺寸与游戏渲染分辨率分别记录。

- 找到用户指定的 `.upac`，或在 unityperf 完成采集后通过「文件」保存 `.upac`。记录文件 SHA-256、设备、Unity 版本、运行场景、复现步骤、画质、目标帧率和构建标识。缺少的元数据写为未知。PowerShell 可用 `Get-FileHash -Algorithm SHA256 -LiteralPath <录制路径>`。
- Windows x64 上复制整个 Skill 文件夹即可使用。文件夹内的 `tools/upac.exe` 集成 UPAC v2 解码器、Unity 2022 帧回放、调用树和截图提取以及帧统计。游戏工程仅在回查业务源码时提供，记录与录制构建的版本关系。若回放失败，保留原始错误和文件信息。

## 提取可复查的数据

在 Skill 文件夹打开 PowerShell，替换下方录制路径和目标帧率。先看统计中慢帧的 `position`，再用实际位置替换示例中的 120。JSONL 输出保留在当前目录。

```powershell
$recording = 'D:\captures\battle.upac'
.\tools\upac.exe -file $recording -stats -fps 60
.\tools\upac.exe -file $recording | Set-Content -Encoding utf8 frames.jsonl
if ($LASTEXITCODE) { throw 'UPAC 摘要提取失败' }
.\tools\upac.exe -file $recording -start 120 -count 1 -detail | Set-Content -Encoding utf8 detail.jsonl
if ($LASTEXITCODE) { throw 'UPAC 调用树提取失败' }
```

需要截图时，给详情命令追加 `-image`。匹配成功的帧含 `screenJpegBase64`，可用以下命令保存该帧 JPEG（`detail.jsonl` 为上述单帧窗口）。

```powershell
$row = Get-Content -Encoding utf8 detail.jsonl | Select-Object -Last 1 | ConvertFrom-Json
if ($row.screenJpegBase64) { [IO.File]::WriteAllBytes("$PWD\frame-$($row.position).jpg", [Convert]::FromBase64String($row.screenJpegBase64)) }
```

`-fps` 使用本次目标帧率，未知时省略。工具按 epoch 输出有效帧数、无效时钟数、P50/P95/P99 和最慢帧的位置；有预算时另算严格超限的帧数和比例。分位数采用 nearest rank，数值来自真实样本，同耗时优先选靠前的位置。对特定玩法阶段用提取器的 `-start` 和 `-count` 切出同场景片段再统计。

摘要和详情每行一个 JSON 对象，首行是录制元信息。PowerShell 5 的 UTF-8 输出可能带 BOM，用 Python 读取时可选 `utf-8-sig`。长录制的详细树按小窗口处理。上下文中只保留统计、证据行和必要子树。保留完整精度用于计算，展示时再四舍五入。

## 字段和计算口径

| 数据 | 读取方式 |
| --- | --- |
| 帧定位 | `position` 是完整回放结果中从 0 开始的数组位置。`frameIndex` 是 Player 帧号。报告同时给出 `clockEpoch`、`frameIndex`、`position`。同一文件可有多个时钟段，帧号可能重置或跳号。以顶层字符串 `frameIndex` 为准，便于跨工具保留整数精度。 |
| 帧间隔 | 当 `playerEndNs > playerStartNs`，用整数相减后除以 `1e6` 得到毫秒。同一 epoch 内可与该段锚点相减得到相对时间。`metrics.timestampMs` 在 PD3U 解码时生成，分析事件间隔时使用 Player 时钟。跨 epoch、跳号和时钟无效的区间须标注。 |
| CPU、FPS、GPU | `cpuTimeMs` 优先取主线程根样本耗时，在缺少根样本时使用映射回退值。`fps` 从帧间隔推导。用帧间隔衡量掉帧，用主线程、渲染线程、GPU 和等待样本定位瓶颈。`gpuTimeMs` 缺少数据时可能为 0。`renderMs` 取 CPU 渲染线程计数器，`gpuTimeMs` 取 GPU 帧计数器。 |
| 样本和线程 | `threads[].id/name` 标识线程。主线程 ID 归一化为 `main`，工作线程用 `(clockEpoch, thread.id)` 区分。样本包含 `name`、`category`、`totalMs`、`selfMs`、`calls`、`startMs`、`gcAllocKB`、可选 `callstack` 和 `children`。`totalMs` 含子节点，`selfMs` 是自身耗时。 |
| Timeline | 样本相对帧起点的开始时间为 `thread.offsetMs + sample.startMs`，结束为开始时间加 `sample.totalMs`。在同一 epoch 的帧间比较时，再加该帧相对锚点的 `playerStartNs` 偏移。保留负偏移和线程重叠。跨线程耗时求和属于累计工作量。 |
| 内存与 GC | 按随包提供的[内存字段位表](memory-counters.md)检查 `memoryCounterMask`，区分真实 0 和未提供。分别报告帧级 GC 分配、样本分配、GC 执行耗时、已用内存和预留内存。明确使用的字段，例如 `gcAllocatedFrameKB` 或 `gcAllocKB`。聚合前核对每个字段的采集口径，避免父子样本重复计数。 |
| 截图 | `capture.Screen.Nearest(epoch, playerEndNS)` 仅在有时钟映射和同段截图且满足容差时返回有效匹配。`screenDeltaMs` 为截图时间减对齐后的帧结束时间，正值表示截图更晚。当前容差为流式 ±250 ms、单次截图 ±750 ms。需要图像时对证据帧使用 `-image`，并在报告中附偏差。 |

完整回放可绕过 UI 保留窗口。使用桌面服务对照时，从 `Status().Retained` 获取可查询范围，结合 `FrameAt` 验证 UI 位置与 Player 帧号。`FrameTimeline(position, frameIndex)` 同时校验这两个值。`FrameHeavyProfile(start, end)` 的参数是**保留窗口内**的零基位置，首尾包含。完整回放与 UI 的位置需通过帧号和时钟段交叉核对。

当前解码器在 `internal/unityprofiler/hierarchy.go` 对单次构建的样本树使用 8192 节点预算。回放成功也可能包含流恢复产生的帧缺口。样本计数以可见树为范围，接近节点上限时标注潜在裁剪。样本 `gcAllocKB` 已向祖先汇总，可比较同层分支或单条路径。

## 定位热点并回查源码

1. 先描述覆盖范围和复现条件。统计有效样本数、缺口、各时钟段和各计数器覆盖率。按用户指定片段和同场景基线比较帧间隔的 P50、P95、P99、最大值与预算超限数，说明分位算法及分母。帧预算由目标 FPS 确定，例如 60 FPS 对应约 16.67 ms。基线和异常均选若干代表帧。
2. 对异常窗口提取主线程、渲染线程和相关工作线程。区分频繁的小调用、单次长调用和等待。沿 Timeline 核对线程之间的先后与重叠，结合预算、GPU 计数器和 `WaitFor…` 等标记评估瓶颈，标明缺失指标对结论的影响。
3. 做 Left Heavy 时，按线程与完整调用路径分组合并。平铺统计按线程与 `(name, category)` 分组，并保留主要父路径。分别累加 `calls`、`totalMs`、`selfMs`。需要对齐 unityperf 的 Left Heavy 时采用 `max(1, calls)`，见 `services/heavyprofile.go`。调用均值用累计耗时除以累计调用次数，每帧均摊用窗口有效帧数。递归或嵌套同名样本的 inclusive 总耗时可能重复覆盖同一时间，报告瓶颈优先看 self 和完整路径。
4. 在游戏工程根目录用 PowerShell `Get-ChildItem Assets,Packages -Recurse -File -Filter *.cs | Select-String -Encoding utf8 -SimpleMatch '<marker 完整名>'` 找定义，如 `ProfilerMarker`、`Profiler.BeginSample` 或封装。方法型 marker 继续搜索类、方法、调用点、程序集和重载。沿捕获到的父子路径核对调用链，也检查协程、`MoveNext`、异步状态机、事件、Unity 生命周期和 Jobs 调度点。涉及第三方包时核对包版本和实际包源码。
5. 每个结论附一个可复现证据链。给出录制文件及帧定位、线程、完整 marker 路径、耗时与调用次数、基线差异，以及源码文件和行号。用「已证实」「可能」「缺少证据」标记归因强度。缺少托管调用栈、符号或源码版本匹配时，列出已匹配位置和待验证的候选，给出能区分候选的最小复测，例如为候选分支增设 `ProfilerMarker`，在同场景同设置下重录并复算相同指标。

## 按问题选择证据

- **持续掉帧或偶发卡顿**。同时展示典型帧和尾部慢帧。分别在正常与慢帧窗口按每帧 self 毫秒、调用次数和出现帧占比排序。高频轻调用看单次 self 与总次数，单帧尖峰看相邻帧和父路径。报告占比时用同一帧的耗时除以该帧间隔，再汇总各帧占比；写明缺失树的帧数。区分主动工作和等待，检查与等待区间重叠的线程。`Gfx.WaitForPresentOnGfxThread` 也可能涉及帧率限制或 VSync，需结合同帧 GPU 计时或图形工具证据判断。
- **GC 或资源增长**。把分配峰值、回收执行、回收后内存低点放到同一时间轴。比较重复进入和退出同一场景后的低点，结合可用的纹理、网格、材质、对象计数缩小范围。帧级计数器支持增长趋势判断；定位具体对象和引用链时，再用同一 Player、同一阶段的 Memory Profiler 快照。渲染提交异常可按同一复现场景用 Frame Debugger 核对批次和材质切换，并注明两次采集的对应关系。
- **优化前后比较**。同设备、画质、帧率设置及采样模式下按玩法阶段对齐，记录两边有效帧数、时长、预热、加载和收尾是否入样。分别列出 P50/P95/P99、超预算率和关键路径每帧 self，再给出“优化后减优化前”的毫秒差或百分点差。Deep Profile 可用于找调用路径，用相同采样模式复核耗时。小窗口的 P99 常对应最大值，应附对应帧及重复次数。
- **报告和归因**。优先展示能解释帧预算超限或前后差异的热点。每项带证据帧、线程、marker 路径、self/total、调用次数与下一次可验证的预测。说明代码属于游戏业务、基础框架或引擎/第三方。没有 Unity 引擎源码时，归因到可见 marker 或已核实的游戏调用点。需要分享大量热点时，可生成可排序的离线 HTML 表；简短诊断直接给出结论和证据帧即可。

分析方法参考 [profiler-dump-analyze](https://github.com/boomlulu/unity-performance-dump)、[UnityProfilerTool](https://github.com/clove7111/UnityProfilerTool)、[Unity-Skills](https://github.com/Besty0728/Unity-Skills) 和 [Unity 2022.3 Profile Analyzer](https://docs.unity3d.com/2022.3/Documentation/Manual/com.unity.performance.profile-analyzer.html)。离线运行只需本文件夹和 Windows x64。`scripts` 保留工具源码和已有 JSONL 的可选 Python 统计入口。
