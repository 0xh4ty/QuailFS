package main

import (
	"embed"
	"fmt"
	"github.com/0xh4ty/quailfs/internal/app"
	"github.com/0xh4ty/quailfs/internal/keys"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"os"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	switch {
	case len(os.Args) == 1:
		runClient()

	case os.Args[1] == "--node" && len(os.Args) > 2 && os.Args[2] == "--relay":
		fmt.Println("QuailFS")
		fmt.Println("Starting Node with Relay...")
		app.RunNode(true)

	case os.Args[1] == "--node":
		fmt.Println("QuailFS")
		fmt.Println("Starting Node...")
		app.RunNode(false)

	case os.Args[1] == "--generate-mnemonic":
		fmt.Println("Generating Mnemonic")
		keys.GenerateMnemonic()

	default:
		fmt.Println("Unknown argument:", os.Args[1])
		fmt.Println("Usage: quailfs [--node] [--node --relay] [--generate-mnemonic]")
		os.Exit(1)
	}
}

func runClient() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:      "quailfs",
		Width:      1280,
		Height:     800,
		Fullscreen: true,

		AssetServer: &assetserver.Options{
			Assets: assets,
		},

		OnStartup: app.startup,

		Bind: []any{
			app,
		},
	})

	if err != nil {
		fmt.Println("Error:", err.Error())
	}
}
