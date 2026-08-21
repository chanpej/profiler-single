# UnityPerfAgent

UnityPerfAgent 是一款独立于 Unity Editor 的性能分析工具：经逆向分析实现与 Editor Profiler 同源的真机采集协议，连接 Unity 2022 Development Build，采集 CPU / Memory / Rendering 等模块数据，并通过 ACP（Agent Client Protocol）接入 LLM Agent，实现自动化性能诊断。

> 当前仅支持 **Unity 2022**（其他版本暂不考虑）。握手阶段会校验 Player 上报的 Unity 版本。生产包无法 Attach Profiler。

## 技术栈

| 层 | 选型 |
| --- | --- |
| 后端 | Go 1.25 + [Wails v3.0.0-beta.8](https://github.com/wailsapp/wails) |
| 前端 | React 18 + TypeScript + Vite |
| 图标 | [lucide-react](https://lucide.dev/) |
| 虚拟化 | [@tanstack/react-virtual](https://tanstack.com/virtual)（十万级帧数据流畅滚动） |
| LLM 接入 | [ACP（Agent Client Protocol）](https://agentclientprotocol.com) via [acp-go-sdk](https://github.com/coder/acp-go-sdk) |

## 架构

```
internal/adbforward     USB 网络转发：adb devices + adb forward tcp:0 localabstract:Unity-{包名}
internal/unityprofiler  Unity 2022 Editor Profiler 协议（逆向实现）
                        USB PlayerConnection：GUID 报文 + Editor 握手（UnityEditor）+ Profiler enable
                        协议单测假 Player：TCP GeneralConnection magic 0xDAFEDADA
                        Connect=1 / Disconnect=2 / ProfilerFrame=0x20
internal/profilerproxy  可复用的透明 PlayerConnection 代理，抓取 Editor 接收的原始 PD3U
cmd/profilerproxy       透明代理命令，用于生成同一 Editor 会话的 .uppc / .data 对照样本
internal/upac           原生会话存档：握手元信息 + 原始 PD3U 分块的版本化容器
internal/protocol       Profiler 模块快照编码（FrameData 164 字节，兼容 v1 68 字节）
                        作为 ProfilerFrame 载荷；握手校验 Unity 2022
internal/device         Profiler 客户端封装
internal/collector      环形缓冲会话存储（O(1) 追加、范围读取、按索引取帧、聚合统计）
internal/analyzer       规则化诊断：帧尖峰 / GC 尖峰 / 低帧率 / DrawCall / 内存增长
internal/llm            ACP 客户端：stdio 拉起外部 LLM Agent（隐藏控制台窗口）
internal/procutil       外部进程封装：Windows 下 CREATE_NO_WINDOW + HideWindow
services                Wails 服务层（连接管理、组播发现、数据查询、事件节流推送）
frontend                React UI：Profiler（对齐 Unity Profiler）/ 帧数据 / 诊断 / 设置
```

## 性能设计

- 帧数据采用固定长度二进制编码，单帧负载 164 字节（v1 兼容 68 字节）；
- 会话存储为固定容量环形缓冲（默认 10.8 万帧），内存有上界；
- 后端事件按 250ms 节流推送，前端按 500ms 节流刷新；
- 帧表格全虚拟化渲染，数据按 200 帧分块从后端懒加载；
- Profiler 图表用 Canvas 直绘堆叠柱，无图表库开销。

## 数据采集（对齐 Unity Profiler 模块）

采集通道与 Editor「Window → Analysis → Profiler」USB Attach 相同：Development Build 在 USB 调试开启后，工具以 `UnityEditor` 身份连接 `localabstract:Unity-{包名}`，接收 CPU / Memory / Rendering 等模块计数器。

每帧包含：

| 模块 | 字段 |
| --- | --- |
| 基础 | FrameIndex、Timestamp、FPS、CPU/GPU/MainThread/Render 耗时、GC Alloc |
| CPU Usage | Rendering / Scripts / Physics / Animation / GC / VSync / UI / Others（ms） |
| Memory | Used/Reserved Total、GC、Gfx、Audio、Video、Profiler、System（MB） |
| Rendering | Draw Calls、SetPass、Batches、Triangles、Vertices、Shadow Casters |
| Audio | Playing Sources、Audio Voices |
| Physics | Active Rigidbodies、Colliders |
| UI | Batches、Vertices |

外部程序（ACP Agent 等 exe）一律通过 `procutil` 启动，Windows 下隐藏控制台黑窗。

## 存档格式

- **`.upac`**：UnityPerfAgent 的正式会话格式。保存握手元信息和完整的原始 PD3U
  报文体；打开时使用当前 Decoder 重放，因此后续解码能力升级后仍可重新分析旧存档。
  实时采集只能保存为此格式。
- **`.data`**：Unity Editor Profiler 的私有离线格式，仅支持导入读取。工具不会从
  PD3U 或 UI 摘要帧伪造 `.data`，以避免生成 Editor 无法打开的文件。
- **`.uppc`**：透明抓包代理的研究格式，仅用于保存同一 Editor 会话的原始 PD3U
  对照数据，不作为常规用户存档。

## Profiler UI

主视图还原 Unity Profiler 2022 的交互：

- 工具栏：Record 状态、Deep Profile / Call Stacks（需 Editor 插桩，独立工具中禁用）、Clear、帧前进/后退、Current、当前帧耗时与 FPS；
- 模块开关：CPU Usage、GPU Usage、Rendering、Memory、Audio、Video、Physics、UI；
- 堆叠时间轴：点击选帧，录制时自动跟随最新帧；
- 详情面板：CPU Hierarchy / Timeline、Memory Simple、Rendering/Audio/Physics/UI 计数器表。

## LLM 诊断（ACP）

在“设置”页配置任意支持 ACP 的 Agent，例如：

- Gemini CLI：命令 `gemini`，参数 `--experimental-acp`
- Claude Code（ACP 适配器）：命令 `claude-code-acp`

诊断时会把规则分析报告（JSON）交给 Agent，返回中文诊断结论与优化建议。

### MCP 性能数据工具

每次创建 Agent 会话时，应用会通过 ACP 的 `session/new.mcpServers` 注入内置
stdio MCP Server。Agent 可按需查询当前内存中的实时采集数据、已加载的 `.upac`
存档或 Unity 2022 `.data` 帧数据，而不是只依赖预先生成的规则报告：

- `performance_session_summary`：连接状态、聚合指标与规则分析；
- `performance_frames`：范围帧摘要（单次最多 200 帧）；
- `performance_frame` / `performance_cpu_hierarchy`：单帧完整计数器及 CPU 层级；
- `performance_analysis`：重新运行内置规则分析。

MCP bridge 只监听 `127.0.0.1`，并使用每次运行生成的令牌鉴权。菜单栏的
**Agent Log** 会打开独立原生窗口，显示 Agent 对话和 MCP 请求/响应的完整 payload。

## 开发

```bash
# 依赖：Go >= 1.25、Node >= 20、wails3 CLI v3.0.0-beta.8
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8

wails3 dev     # 开发模式
wails3 build   # 产物输出到 bin/unityperfagent.exe
```

## 测试

```bash
go test ./...            # Go 后端测试（协议/采集/分析/设备/ACP/服务层）
cd frontend && npm test  # 前端测试（vitest + testing-library）
```

## 真机接入

目标设备需运行 **Unity 2022 Development Build**（测试包为 Unity 2022.3.47f1），并开启 USB 调试。

1. **USB（adb forward）**：与官方 Profiler 相同，执行  
   `adb forward tcp:<本机端口> localabstract:Unity-{应用包名}`  
   （不是把设备 `tcp:55000` 当 USB 通道）。工具会自动从 `/proc/net/unix` 和前台应用解析 `Unity-{bundleId}`。界面点「USB 连接」。  
   若 Player 先推送 USB 宣告：`[Debug]` 只表示脚本调试，`Flags` bit1 为 SupportsProfile。Development Build 即使 `[Debug] 0` 也可以 Attach。
2. 工具以 `UnityEditor` 身份完成 PlayerConnection 握手并启用 Profiler，仅接受 Unity 2022；
3. 之后 Player 按帧推送 Profiler 模块计数器快照（CPU 分类、内存分项、渲染/音频/物理/UI）。

生产包无法 Attach Profiler。

## Unity Editor 对照抓包

`cmd/profilerproxy` 是长期保留的透明代理：它逐条转发 Unity Editor 与 Player
之间的 PlayerConnection 报文，只记录 Player → Editor 的 `MsgIDProfilerData`
报文体。输出的 `.uppc` 是本工具的 PD3U 对照格式，不是 Unity `.data`。

1. 先将 Android Player 转发到本机端口，例如：

   ```bash
   adb forward tcp:55000 localabstract:Unity-<包名>
   ```

2. 启动代理：

   ```bash
   go run ./cmd/profilerproxy -listen 127.0.0.1:55018 -target 127.0.0.1:55000 -out test/same-session.uppc
   ```

3. 在 Unity Editor 的 **Profiler → Attach to Player → Enter IP** 中填写
   `127.0.0.1:55018`，录制所需帧，然后在该 Profiler 窗口执行 **Save**，保存为
   `same-session.data`；
4. 断开 Editor，代理会写出 `same-session.uppc`。请保留这两个文件，用于验证
   PD3U Decoder 与 Unity Editor 对同一会话的帧对齐关系。

代理不会修改或重编码任何 PlayerConnection 报文，也不会生成 `.data`。
