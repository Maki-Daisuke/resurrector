package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

var defaultCommandExtensions = []string{".exe", ".cmd", ".bat", ".com", ".ps1", ".vbs"}

func commandExtensions() []string {
	raw := strings.TrimSpace(os.Getenv("PATHEXT"))
	if raw == "" {
		return append([]string(nil), defaultCommandExtensions...)
	}

	parts := strings.Split(raw, string(os.PathListSeparator))
	seen := make(map[string]struct{}, len(parts))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		ext := strings.ToLower(strings.TrimSpace(part))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		if _, ok := seen[ext]; ok {
			continue
		}
		seen[ext] = struct{}{}
		result = append(result, ext)
	}

	if len(result) == 0 {
		return append([]string(nil), defaultCommandExtensions...)
	}

	return result
}

func commandFileDialogPattern() string {
	extensions := commandExtensions()
	patterns := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		patterns = append(patterns, "*"+ext)
	}
	return strings.Join(patterns, ";")
}

// GetCommandExtensions returns the executable/script extensions accepted by this Windows environment.
func (a *App) GetCommandExtensions() []string {
	return commandExtensions()
}

// SelectCommandPath opens a native file dialog for selecting or typing a command path.
func (a *App) SelectCommandPath(current string) (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("app context is not initialized")
	}

	defaultFilename := strings.TrimSpace(current)
	if defaultFilename != "" {
		defaultFilename = filepath.Clean(defaultFilename)
	}

	selected, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:           "Select Command (or type a path in File name)",
		DefaultFilename: defaultFilename,
		Filters: []runtime.FileFilter{
			{DisplayName: "Command Files", Pattern: commandFileDialogPattern()},
			{DisplayName: "All Files", Pattern: "*"},
		},
	})
	if err != nil {
		return "", err
	}

	return selected, nil
}
