package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Data source: if HYDRASCALE_SOCKET points at a daemon control socket
	// (locally, or an `ssh -L` forward of a remote daemon's api.sock), talk to
	// it live; otherwise fall back to built-in fixtures so the app runs
	// standalone with no daemon attached.
	var src DataSource = newMockSource()
	if sock := os.Getenv("HYDRASCALE_SOCKET"); sock != "" {
		src = newSocketSource(sock)
	}

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
