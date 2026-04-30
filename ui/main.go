package main

import (
	"embed"
	"fmt"
	"log/slog"
	"os"

	"resurrector/util"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
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
	runtimeFlags, err := util.ParseRuntimeFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse runtime flags: %v\n", err)
		os.Exit(1)
	}
	closeLogWriter, err := util.ConfigureLogger(runtimeFlags.LogFile, runtimeFlags.LogFormat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to configure logger, falling back to stderr text: %v\n", err)
		closeLogWriter, err = util.ConfigureLogger("", "text")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to configure fallback logger: %v\n", err)
			os.Exit(1)
		}
	}
	if closeLogWriter != nil {
		defer closeLogWriter()
	}

	slog.Info("starting Resurrector",
		slog.String("component", "main"),
		slog.String("binary", "ui"),
		slog.String("version", Version),
	)

	// Create an instance of the app structure
	app := NewApp(runtimeFlags.ConfigPath)

	// Create application with options
	err = wails.Run(&options.App{
		Title:  fmt.Sprintf("Resurrector v%s", Version),
		Width:  950,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
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
