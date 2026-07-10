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
	// Data source: fixtures for now so the app runs standalone. The SSH-tunnel
	// socket source is selected here once it lands (M3).
	var src DataSource = mockSource{}

	app := NewApp(src)

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
