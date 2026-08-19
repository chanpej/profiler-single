package main

import (
	"context"
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"unityperfagent/internal/mcp"
	"unityperfagent/services"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// 注册事件类型，绑定生成器会为前端生成强类型 API。
	application.RegisterEvent[services.TickEvent](services.TickEventName)
	application.RegisterEvent[services.ImportProgressEvent](services.ImportProgressEventName)
	application.RegisterEvent[services.AgentChunkEvent](services.AgentChunkEventName)
}

func main() {
	// The desktop executable is also the stdio MCP transport launched by ACP
	// Agents. It does not initialise Wails in this mode.
	if len(os.Args) == 2 && os.Args[1] == "mcp" {
		if err := mcp.Serve(context.Background(), os.Stdin, os.Stdout, os.Getenv("UNITY_PERF_MCP_ENDPOINT"), os.Getenv("UNITY_PERF_MCP_TOKEN")); err != nil {
			log.Print(err)
		}
		return
	}

	app := application.New(application.Options{
		Name:        "UnityPerfAgent",
		Description: "独立于 Unity Editor 的真机性能分析与 LLM 自动诊断工具（Unity 2022）",
		Services: []application.Service{
			application.NewService(services.NewPerfService()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "UnityPerfAgent",
		Width:            1440,
		Height:           900,
		MinWidth:         1080,
		MinHeight:        680,
		Frameless:        true,
		BackgroundColour: application.NewRGB(10, 12, 16),
		URL:              "/",
		Windows: application.WindowsWindow{
			Theme:                      application.Dark,
			NonClientRegionSupport:     true,
			WebView2CompositionHosting: true,
		},
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
