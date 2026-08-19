package services

import "github.com/wailsapp/wails/v3/pkg/application"

const agentLogWindowName = "agent-log"

// OpenAgentLog shows the interaction history in a separate native WebView
// window, so inspecting full MCP payloads never obscures the profiler view.
func (p *PerfService) OpenAgentLog() {
	app := application.Get()
	if existing, ok := app.Window.GetByName(agentLogWindowName); ok {
		existing.Show().Focus()
		return
	}
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             agentLogWindowName,
		Title:            "UnityPerfAgent - Agent Log",
		Width:            980,
		Height:           720,
		MinWidth:         640,
		MinHeight:        420,
		BackgroundColour: application.NewRGB(10, 12, 16),
		URL:              "/?agent-log=1",
		Windows: application.WindowsWindow{
			Theme: application.Dark,
		},
	})
}
