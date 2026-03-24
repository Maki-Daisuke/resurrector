package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// Custom logger writing to stderr
type stderrLogger struct{}

func (l *stderrLogger) Print(message string)   { os.Stderr.WriteString(message + "\n") }
func (l *stderrLogger) Trace(message string)   { os.Stderr.WriteString("[TRACE] " + message + "\n") }
func (l *stderrLogger) Debug(message string)   { os.Stderr.WriteString("[DEBUG] " + message + "\n") }
func (l *stderrLogger) Info(message string)    { os.Stderr.WriteString("[INFO] " + message + "\n") }
func (l *stderrLogger) Warning(message string) { os.Stderr.WriteString("[WARN] " + message + "\n") }
func (l *stderrLogger) Error(message string)   { os.Stderr.WriteString("[ERROR] " + message + "\n") }
func (l *stderrLogger) Fatal(message string)   { os.Stderr.WriteString("[FATAL] " + message + "\n") }

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Resurrector",
		Width:  650,
		Height: 500,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
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
