UnityPerfAgent是一款独立于Unity Editor的性能分析工具，采用完全自研的独立真机Unity游戏的性能数据连接、采集、分析实现（获取Unity Editor Profiler一样的采集数据），并接入LLM能力，能进行自动化的性能分析诊断。

# 技术目的

- 独立Unity Editor
- 自研的真机游戏性能数据连接、采集、分析实现
- 接入LLM能力，能进行自动化的性能分析诊断
- 超高性能的GUI展示与操作

# 技术选型

- go
- wails3 https://github.com/wailsapp/wails/releases#release-v3.0.0-beta.8
- react and typescript

# 软件UI设计

- 简洁现代风格，偏向工具类软件
- 多使用图标，少使用文字，使用成熟的图标库，不要自绘

# 性能要求

- 必须保持高性能
- 数据显示要考虑虚拟化
