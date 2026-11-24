# unityperf

unityperf 是一款独立于 Unity Editor 的性能分析工具：经逆向分析实现与 Editor Profiler 同源的真机采集协议，连接 Unity 2022 Development Build，采集 CPU / Memory / Rendering 等模块数据，并支持本地规则分析。

> 当前仅支持 **Unity 2022**（其他版本暂不考虑）。握手阶段会校验 Player 上报的 Unity 版本。生产包无法 Attach Profiler。

## 技术栈

| 层 | 选型 |
| --- | --- |
| 后端 | Go 1.25 + [Wails v3.0.0-beta.8](https://github.com/wailsapp/wails) |
| 前端 | React 18 + TypeScript + Vite |
| 图标 | [lucide-react](https://lucide.dev/) |
| 虚拟化 | [@tanstack/react-virtual](https://tanstack.com/virtual)（十万级帧数据流畅滚动） |

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
internal/procutil       外部进程封装：Windows 下 CREATE_NO_WINDOW + HideWindow
services                Wails 服务层（连接管理、组播发现、数据查询、事件节流推送）
frontend                React UI：Profiler（对齐 Unity Profiler）/ 帧数据 / 诊断
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

外部程序一律通过 `procutil` 启动，Windows 下隐藏控制台黑窗。

## 存档格式

- **`.upac`**：unityperf 的正式会话格式。保存握手元信息和完整的原始 PD3U
  报文体；打开时使用当前 Decoder 重放，因此后续解码能力升级后仍可重新分析旧存档。
- **`.data`**：Unity Editor Profiler 的离线格式。通过「文件 → 导出 Unity .data」选择
  与录制兼容的 Unity 或团结 2022 Editor 可执行文件。工具在临时项目中启动该 Editor，
  由其原生保存并验证文件可重新加载。导出需要单个 Player 会话的原始录制，不含截图，
  并受 Editor 的帧历史上限约束。
- **`.uppc`**：透明抓包代理的研究格式，仅用于保存同一 Editor 会话的原始 PD3U
  对照数据，不作为常规用户存档。

## Profiler UI

主视图还原 Unity Profiler 2022 的交互：

- 工具栏：Record 状态、Deep Profile / Call Stacks（需 Editor 插桩，独立工具中禁用）、Clear、帧前进/后退、Current、当前帧耗时与 FPS；
- 模块开关：CPU Usage、GPU Usage、Rendering、Memory、Audio、Video、Physics、UI；
- 堆叠时间轴：点击选帧，录制时自动跟随最新帧；
- 详情面板：CPU Hierarchy / Timeline、Memory Simple、Rendering/Audio/Physics/UI 计数器表。

## 开发

```bash
# 依赖：Go >= 1.25、Node >= 20、wails3 CLI v3.0.0-beta.8
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8

wails3 dev     # 开发模式
wails3 build   # 产物输出到 bin/unityperf.exe
```

## 测试

```bash
go test ./...            # Go 后端测试（协议/采集/分析/设备/服务层）
cd frontend && npm test  # 前端测试（vitest + testing-library）
```

## 真机接入

目标设备需运行 **Unity 2022 Development Build**（测试包为 Unity 2022.3.47f1）。使用 USB 时需开启 USB 调试。

1. **USB（adb forward）**：与官方 Profiler 相同，执行  
   `adb forward tcp:<本机端口> localabstract:Unity-{应用包名}`  
   （不是把设备 `tcp:55000` 当 USB 通道）。工具会自动从 `/proc/net/unix` 和前台应用解析 `Unity-{bundleId}`。界面点「USB 连接」。  
   若 Player 先推送 USB 宣告：`[Debug]` 只表示脚本调试，`Flags` bit1 为 SupportsProfile。Development Build 即使 `[Debug] 0` 也可以 Attach。
2. **手机 IP 直连**：电脑和手机在可互通的局域网内时，输入手机 IP 并点「IP 直连」。工具尝试 `55000–55016/TCP`，选择首个通过 Profiler 握手的 Player。
3. 工具以 `UnityEditor` 身份完成 PlayerConnection 握手并启用 Profiler，仅接受 Unity 2022；
4. 之后 Player 按帧推送 Profiler 模块计数器快照（CPU 分类、内存分项、渲染/音频/物理/UI）。

生产包无法 Attach Profiler。

## 团结 Autoconnected Player

unityperf 可直接接收基于 Unity 2022 的团结微信小游戏实时采样，采集电脑可独立运行 unityperf。实测版本为团结 `2022.3.62t13`。

1. 构建微信小游戏 Development Build 时启用 Autoconnect Profiler，并把 Profiler 目标设为采集电脑的局域网 IP，端口设为 `37100`。
2. 打开 unityperf 即会监听 `37100`，工具栏显示本机 IP 和等待状态。若端口被占用，释放端口后可点 **小游戏自动连接** 重试。
3. 在同一局域网内进入小游戏。若游戏之前已连接团结 Editor，重新进入以建立新的连接。

点击连接工具栏的 **Unity 日志** 查看最近 500 条日志。微信小游戏通过团结 Console 使用的原生 PlayerConnection 通道发送日志，可直接使用已有的 Autoconnect Profiler 开发包。Android USB 连接还可从当前游戏进程的 ADB logcat 读取 `Unity` 日志。

播放器的主线程分块在上报时发生缺失，采集会跳过受损帧，并从下一个可识别的帧边界恢复。日志中的 `MissingMainChunks`、`DiscardedMainChunks` 和 `FrameResyncs` 分别记录缺失分块、等待恢复时跳过的分块和恢复次数。

在 CPU Usage 的 Timeline 中点击采样块，即可查看完整采样路径、总耗时、自身耗时和 Player 上报的符号调用栈。

通过 ADB 连接 Android 手机后，点击「采集截图」开启画面轨道。小游戏使用唯一一台在线的 ADB 手机。支持 Android `screenrecord` 帧流时优先使用低分辨率连续采集，最高保留每秒 8 张，否则回退到单张截图。点击缩略图可跳到对应性能帧。画面使用 Player 帧时钟估算同步，最近 1440 张附随 `.upac` 保存。

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
