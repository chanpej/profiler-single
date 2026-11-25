# unityperf · Unity 2022 真机性能分析

**本项目改编自 librashuai 的 UnityPerfAgent。** 原项目提供了独立桌面 Profiler、Unity Player 通信与性能分析的基础。本仓库沿用其 Git 历史，侧重 Android 真机录制、团结微信小游戏接入，以及把一次采样保存为 `.upac`，交给同事或分析 Skill 离线复查。感谢上游作者和贡献者。[原项目 UnityPerfAgent](https://github.com/librashuai/UnityPerfAgent) · [本项目与上游的共同提交](https://github.com/librashuai/UnityPerfAgent/commit/8d60c534a7e558025c0512820203dd735bf920fd)

## 五分钟录一次真机卡顿

1. 在 Windows x64 电脑上打开 [unityperf.exe](bin/unityperf.exe)。准备 Android SDK Platform-Tools，确认终端能运行 `adb devices -l`。
2. 在手机上启动 **Unity 2022 Development Build**，保持游戏在前台。项目的 Android 测试版本为 Unity `2022.3.47f1`。使用 USB 时，开启 USB 调试并授权这台电脑。
3. 在 unityperf 输入游戏的 Android 包名，点击 **USB 连接**。也可以将电脑和手机接入可互通的局域网，输入手机 IP 后点击 **IP 直连**。直连依赖 Player 开放 Profiler 端口。
4. 在录制期间复现卡顿，然后暂停。在 CPU Usage 图上选中慢帧，打开 **Timeline** 查看主线程、渲染线程和工作线程的时间关系。点击采样块查看完整调用路径、自身耗时和总耗时。
5. 从 **文件** 菜单将录制保存为 `.upac`。重新打开后可继续选帧，也可以将录制交给 [UPAC 离线分析 Skill](skills/upac-performance-analysis/SKILL.md)，按同一口径比较优化前后的帧耗时。

## 可以从录制里看到什么

| 问题 | 查看位置 |
| --- | --- |
| 哪一帧超出预算，时间花在哪个方法？ | CPU Usage → Timeline / Hierarchy。结合调用次数与自身耗时区分高频调用和单次尖峰。 |
| 卡顿时是否发生了 GC 或内存增长？ | 对照同帧的 GC 分配、GC 耗时、已用和预留内存。计数器覆盖取决于 Player 实际上报的数据。 |
| 画面复杂度是否同时变化？ | Rendering 提供 Draw Calls、SetPass、Batches、三角形和顶点计数。支持 ADB 截图时，可开启画面轨道查看慢帧附近的画面及估算时间差。 |
| 两次修改能否离线复查？ | 保留原始 `.upac`，记录设备、场景、画质和目标 FPS。Skill 可按帧窗口提取调用树，并比较 P50、P95、P99 和超预算帧数。 |

## 团结微信小游戏

在基于 Unity 2022 的团结引擎中构建 Development Build，启用 **Autoconnect Profiler**，目标填采集电脑的局域网 IP，端口填 `37100`。启动 unityperf 后确认工具栏处于监听状态，再进入小游戏。已验证的团结版本为 `2022.3.62t13`。连接后可打开 **Unity 日志** 对照游戏事件。

## 录制格式

- **`.upac`** 是日常保存和交换的格式，保留 Player 握手信息、原始 Profiler 数据分块和可选截图轨道，供后续解码回放。
- **`.data`** 用于在 Unity Editor Profiler 中复查。从「文件 → 导出 Unity .data」选择兼容的 Unity 或团结 2022 Editor，工具会启动 Editor 保存并验证结果。该流程需要单个 Player 会话的原始录制，帧数受 Editor 历史窗口限制。
- **`.uppc`** 用于协议对照。开发者可以用 [profilerproxy](cmd/profilerproxy) 转发 Editor 和 Player 之间的通信，保存同一会话的原始 PD3U 数据，与 Editor 导出的 `.data` 比较。

## 从源码构建

桌面后端使用 Go 1.25 和 Wails v3，前端使用 React、TypeScript 和 Vite。Windows 构建需要 Go、Node.js、npm 和 Wails CLI。以下命令从仓库根目录运行。

```powershell
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8
wails3 build
```

生成文件为 `bin/unityperf.exe`。开发时可用 `wails3 dev`。运行回归测试：

```powershell
go test ./...
npm --prefix frontend ci
npm --prefix frontend test
```

采集与解码实现位于 [internal/unityprofiler](internal/unityprofiler)，存档读写位于 [internal/upac](internal/upac)，桌面服务和界面分别位于 [services](services) 和 [frontend](frontend)。
