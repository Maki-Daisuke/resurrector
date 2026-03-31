package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// Custom logger writing to stderr
type stderrLogger struct{}

func (l *stderrLogger) Print(message string)   { log.Print(message) }
func (l *stderrLogger) Trace(message string)   { log.Print("[TRACE] " + message) }
func (l *stderrLogger) Debug(message string)   { log.Print("[DEBUG] " + message) }
func (l *stderrLogger) Info(message string)    { log.Print("[INFO] " + message) }
func (l *stderrLogger) Warning(message string) { log.Print("[WARN] " + message) }
func (l *stderrLogger) Error(message string)   { log.Print("[ERROR] " + message) }
func (l *stderrLogger) Fatal(message string)   { log.Print("[FATAL] " + message) }

func main() {
	configPath := parseFlags()

	// Create an instance of the app structure
	app := NewApp(configPath)

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Resurrector",
		Width:  900,
		Height: 700,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		Bind: []interface{}{
			app,
		},
		Logger:   &stderrLogger{},
		LogLevel: logger.WARNING,
	})

	if err != nil {
		log.Fatal(err)
	}
}

