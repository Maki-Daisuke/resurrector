package main

import (
	"bufio"
	"context"
	"os"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx        context.Context
	configPath string
}

// NewApp creates a new App application struct
func NewApp(configPath string) *App {
	return &App{configPath: configPath}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// domReady is called after the frontend DOM has been loaded.
func (a *App) domReady(ctx context.Context) {
	// Start JSON listener on Stdin in the background.
	// We add a tiny delay to give Svelte's onMount enough time to register its EventsOn listener
	// before we consume the buffered initial state from the OS pipe.
	go func() {
		time.Sleep(200 * time.Millisecond)

		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			runtime.EventsEmit(ctx, "app_state_update", scanner.Text())
		}

		// Core process exited, Stdin pipe closed -> shut down UI
		runtime.Quit(ctx)
	}()
}
