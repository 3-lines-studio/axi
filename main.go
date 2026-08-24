package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[StreamEvent]("ax:event")
}

func main() {
	ax := NewAXService()
	app := application.New(application.Options{
		Name:        "Axi",
		Description: "Native client for Axis",
		Services: []application.Service{
			application.NewService(ax),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	ax.app = app

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "AX",
		Width:            1000,
		Height:           720,
		MinWidth:         360,
		MinHeight:        520,
		BackgroundColour: application.NewRGB(246, 246, 243),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
