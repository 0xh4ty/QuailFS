package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/0xh4ty/quailfs/internal/app"
	"github.com/0xh4ty/quailfs/internal/keys"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	switch {
	case len(os.Args) == 1:
		runClient()

	case os.Args[1] == "--node" && len(os.Args) > 2 && os.Args[2] == "--relay":
		printBanner()
		fmt.Println()
		fmt.Println("----------------------------------------")
		fmt.Println(" Starting Storage Node")
		fmt.Println("----------------------------------------")
		fmt.Println()
		fmt.Println("Relay: ENABLED")
		fmt.Println()
		app.RunNode(true)

	case os.Args[1] == "--node":
		printBanner()
		fmt.Println()
		fmt.Println("----------------------------------------")
		fmt.Println(" Starting Storage Node")
		fmt.Println("----------------------------------------")
		fmt.Println()
		fmt.Println("Relay: DISABLED")
		fmt.Println()
		app.RunNode(false)

	case os.Args[1] == "--generate-mnemonic":
		printBanner()
		fmt.Println()
		fmt.Println("----------------------------------------")
		fmt.Println(" Generating Recovery Mnemonic")
		fmt.Println("----------------------------------------")
		fmt.Println()
		fmt.Println("WARNING: This mnemonic is your recovery secret.")
		fmt.Println("Do not share it or store it in plaintext.")
		fmt.Println("Clear your terminal after recording the mnemonic securely.")
		fmt.Println()

		mnemonic := keys.GenerateMnemonic()

		fmt.Println("----------------------------------------")
		fmt.Println(" Recovery Mnemonic")
		fmt.Println("----------------------------------------")
		fmt.Println()
		fmt.Println(mnemonic)
		fmt.Println()
		fmt.Println("----------------------------------------")

	default:
		printBanner()
		fmt.Println()
		fmt.Println("----------------------------------------")
		fmt.Println(" Unknown Argument")
		fmt.Println("----------------------------------------")
		fmt.Println()
		fmt.Println("Unknown argument:", os.Args[1])
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  quailfs")
		fmt.Println("  quailfs --node")
		fmt.Println("  quailfs --node --relay")
		fmt.Println("  quailfs --generate-mnemonic")
		fmt.Println()
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

func printBanner() {
	fmt.Println(`
 .d88888b.                    d8b 888 8888888888 .d8888b.
d88P" "Y88b                   Y8P 888 888       d88P  Y88b
888     888                       888 888       Y88b.
888     888 888  888  8888b.  888 888 8888888    "Y888b.
888     888 888  888     "88b 888 888 888           "Y88b.
888 Y8b 888 888  888 .d888888 888 888 888             "888
Y88b.Y8b88P Y88b 888 888  888 888 888 888       Y88b  d88P
 "Y888888"   "Y88888 "Y888888 888 888 888        "Y8888P"
       Y8b

   A Distributed Encrypted Backup and Recovery System`)
}
