package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Connection comes from persisted settings (Settings modal), overridable by
	// the HYDRASCALE_SOCKET env var. Defaults to the built-in mock so the app
	// runs standalone with no daemon attached.
	app := NewApp(loadSettings())

	err := wails.Run(&options.App{
		Title:     "Hydrascale",
		Width:     1060,
		Height:    720,
		MinWidth:  760,
		MinHeight: 520,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 13, G: 16, B: 19, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title:   "Hydrascale",
				Message: "Manage multiple Tailscale tailnets.",
			},
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
