package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// wailsSlogLogger adapts Wails logger callbacks to slog.
type wailsSlogLogger struct{}

func (l *wailsSlogLogger) Print(message string) {
	slog.Info(message,
		slog.String("component", "wails"),
	)
}
func (l *wailsSlogLogger) Trace(message string) {
	slog.Debug(message,
		slog.String("component", "wails"),
		slog.String("wails_level", "trace"),
	)
}
func (l *wailsSlogLogger) Debug(message string) {
	slog.Debug(message,
		slog.String("component", "wails"),
		slog.String("wails_level", "debug"),
	)
}
func (l *wailsSlogLogger) Info(message string) {
	slog.Info(message,
		slog.String("component", "wails"),
		slog.String("wails_level", "info"),
	)
}
func (l *wailsSlogLogger) Warning(message string) {
	slog.Warn(message,
		slog.String("component", "wails"),
		slog.String("wails_level", "warning"),
	)
}
func (l *wailsSlogLogger) Error(message string) {
	slog.Error(message,
		slog.String("component", "wails"),
		slog.String("wails_level", "error"),
	)
}
func (l *wailsSlogLogger) Fatal(message string) {
	slog.Error(message,
		slog.String("component", "wails"),
		slog.String("wails_level", "fatal"),
	)
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

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
		Logger:   &wailsSlogLogger{},
		LogLevel: logger.WARNING,
	})

	if err != nil {
		slog.Error("wails run failed",
			slog.String("component", "wails"),
			slog.Any("error", err),
		)
		os.Exit(1)
	}
}
