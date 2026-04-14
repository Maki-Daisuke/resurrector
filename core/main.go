package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/windows"

	"resurrector/util"
)

//go:embed icon.ico
var iconData []byte

var defaultConfigFile = []byte(`["Test Ping App"]
enabled = true
command = "cmd.exe"
args = ["/c", "echo", "Starting ping...", "&&", "ping", "127.0.0.1", "-n", "10"]
cwd = ""
restart_delay_sec = 2
healthy_timeout_sec = 5
hide_window = false
max_retries = -1
`)

var (
	user32         = windows.NewLazySystemDLL("user32.dll")
	procMessageBox = user32.NewProc("MessageBoxW")
)

// showErrorDialog displays a native Windows error dialog with the given title and message.
func showErrorDialog(title, message string) {
	slog.Warn("error dialog shown",
		slog.String("component", "error_dialog"),
		slog.String("message", message),
	)
	titlePtr, _ := windows.UTF16PtrFromString(title)
	messagePtr, _ := windows.UTF16PtrFromString(message)
	const mbOK = 0x00000000
	const mbIconError = 0x00000010
	procMessageBox.Call(0, uintptr(unsafe.Pointer(messagePtr)), uintptr(unsafe.Pointer(titlePtr)), mbOK|mbIconError)
}

// resolveConfigPath returns the absolute path to config.toml.
// If customPath is provided via '-f', it returns that.
// Otherwise, it falls back to ~/.config/resurrector/config.toml and creates it from defaults if missing.
func resolveConfigPath(customPath string) (string, error) {
	if customPath != "" {
		return filepath.Abs(customPath)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting user home dir: %w", err)
	}

	configDir := filepath.Join(home, ".config", "resurrector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config file
		if err := os.WriteFile(configPath, defaultConfigFile, 0644); err != nil {
			return "", fmt.Errorf("writing default config.toml: %w", err)
		}
		slog.Info("created default config.toml",
			slog.String("component", "main"),
			slog.String("path", configPath),
		)
	}

	return configPath, nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	var configFlag string
	flag.StringVar(&configFlag, "f", "", "Path to config.toml")
	flag.Parse()

	// Allow only one core (tray) process. We open a session-local named mutex; the
	// first run creates it, a second run gets ERROR_ALREADY_EXISTS and exits here
	// so we never load config or start the tray twice.
	mutexName, err := windows.UTF16PtrFromString(`Local\Resurrector-core-6d3fe7b9-a7fc-48e8-bc7a-66d205c03b0d`)
	if err != nil {
		showErrorDialog("Resurrector - Error", fmt.Sprintf("Internal error (mutex name):\n\n%v", err))
		os.Exit(1)
	}
	instanceMutex, err := windows.CreateMutex(nil, false, mutexName)
	if err != nil {
		if err == windows.ERROR_ALREADY_EXISTS {
			showErrorDialog(
				"Resurrector",
				"Another copy of Resurrector is already running.\n\nOnly one core instance can run at a time.",
			)
			os.Exit(1)
		}
		showErrorDialog("Resurrector - Error", fmt.Sprintf("Failed to create single-instance lock:\n\n%v", err))
		os.Exit(1)
	}
	defer windows.CloseHandle(instanceMutex)

	configPath, err := resolveConfigPath(configFlag)
	if err != nil {
		showErrorDialog("Resurrector - Error", fmt.Sprintf("Failed to resolve config path:\n\n%v", err))
		os.Exit(1)
	}

	// Initial config load
	apps, err := util.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config.toml:\n\n%v", err)
		showErrorDialog(
			"Resurrector - Configuration Error",
			fmt.Sprintf("Failed to load config.toml:\n\n%v", err),
		)
		os.Exit(1)
	}

	// State change channel for UI notifications
	stateChan := make(chan MonitorStatus, 100)
	onStateChange := func(status MonitorStatus) {
		select {
		case stateChan <- status:
		default:
			// Channel full — drop oldest to avoid blocking
			slog.Warn("state channel full, dropping update",
				slog.String("component", "state_change"),
				slog.String("app", status.Name),
			)
		}
	}

	// Create reconciler and perform initial reconciliation
	reconciler := NewReconciler(onStateChange)
	reconciler.Reconcile(apps)
	slog.Info("initial reconciliation complete",
		slog.String("component", "main"),
		slog.Int("entries", len(apps)),
	)

	// Forward state changes to the UI process
	go func() {
		for status := range stateChan {
			if ui := GetCurrentUI(); ui != nil {
				ui.SendState(status)
			}
		}
	}()

	// Start fsnotify watcher for config.toml
	go watchConfig(configPath, reconciler)

	// Start tray loop (blocks until quit)
	RunSystray(iconData, func() {
		err := ShowUI(reconciler, configPath)
		if err != nil {
			slog.Error("failed to show UI",
				slog.String("component", "main"),
				slog.Any("error", err),
			)
		}
	}, func() {
		slog.Info("quit requested, stopping all monitors",
			slog.String("component", "main"),
		)
		reconciler.StopAll()
		os.Exit(0)
	})
}

// watchConfig uses fsnotify to watch config.toml for changes and triggers
// reconciliation with debouncing.
func watchConfig(configPath string, reconciler *Reconciler) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("failed to create fsnotify watcher",
			slog.String("component", "watcher"),
			slog.Any("error", err),
		)
		return
	}
	defer watcher.Close()

	// Watch the directory (not the file) to handle atomic renames.
	// fsnotify can miss events on the file itself when editors do "write-to-temp + rename".
	dir := filepath.Dir(configPath)
	if err := watcher.Add(dir); err != nil {
		slog.Error("failed to watch directory",
			slog.String("component", "watcher"),
			slog.String("dir", dir),
			slog.Any("error", err),
		)
		return
	}

	slog.Info("watching config for changes",
		slog.String("component", "watcher"),
		slog.String("path", configPath),
	)

	const debounceDelay = 500 * time.Millisecond
	var debounceTimer *time.Timer

	filename := filepath.Base(configPath)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Only react to changes to our config file
			if filepath.Base(event.Name) != filename {
				continue
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) {
				continue
			}

			// Debounce: reset timer on each event
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounceDelay, func() {
				reloadAndReconcile(configPath, reconciler)
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			slog.Error("watcher error",
				slog.String("component", "watcher"),
				slog.Any("error", err),
			)
		}
	}
}

// reloadAndReconcile attempts to reload config.toml and reconcile.
// If the config is invalid, it logs the error and keeps the current state.
func reloadAndReconcile(configPath string, reconciler *Reconciler) {
	const maxRetries = 3
	const retryDelay = 200 * time.Millisecond

	var apps map[string]*util.App
	var err error

	// Retry logic for "Access Denied" / "File in use" errors from atomic writes
	for attempt := range maxRetries {
		apps, err = util.LoadConfig(configPath)
		if err == nil {
			break
		}
		slog.Warn("config reload attempt failed",
			slog.String("component", "reload"),
			slog.Int("attempt", attempt+1),
			slog.Any("error", err),
		)
		time.Sleep(retryDelay)
	}

	if err != nil {
		// Invalid config — keep current state, log error
		slog.Error("config reload failed after retries, keeping current state",
			slog.String("component", "reload"),
			slog.Int("attempts", maxRetries),
			slog.Any("error", err),
		)
		return
	}

	slog.Info("config reloaded, reconciling",
		slog.String("component", "reload"),
		slog.Int("entries", len(apps)),
	)
	reconciler.Reconcile(apps)
}
