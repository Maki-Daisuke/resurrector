package main

import (
	_ "embed"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"

	"resurrector/util"
)

//go:embed icon.ico
var iconData []byte

var (
	user32         = windows.NewLazySystemDLL("user32.dll")
	procMessageBox = user32.NewProc("MessageBoxW")
)

// showErrorDialog displays a native Windows error dialog with the given title and message.
func showErrorDialog(title, message string) {
	titlePtr, _ := windows.UTF16PtrFromString(title)
	messagePtr, _ := windows.UTF16PtrFromString(message)
	const mbOK = 0x00000000
	const mbIconError = 0x00000010
	procMessageBox.Call(0, uintptr(unsafe.Pointer(messagePtr)), uintptr(unsafe.Pointer(titlePtr)), mbOK|mbIconError)
}

func main() {
	// Parse config
	cfg, err := util.LoadConfig("config.toml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config.toml:\n\n%v", err)
		showErrorDialog(
			"Resurrector - Configuration Error",
			fmt.Sprintf("Failed to load config.toml:\n\n%v", err),
		)
		os.Exit(1)
	}

	stateChan := make(chan *AppInfo, 100)
	mgr := NewManager(cfg, stateChan)

	// Start monitoring apps that are enabled by default
	mgr.StartAll()

	// Dispatch state updates to the UI, if active
	go func() {
		for app := range stateChan {
			// Find and update the state in the manager's slice
			for i, a := range mgr.Apps {
				if a.Config.Name == app.Config.Name {
					mgr.Apps[i] = app
					break
				}
			}

			if ui := GetCurrentUI(); ui != nil {
				ui.SendState(app)
			}
		}
	}()

	// Start tray loop
	RunSystray(iconData, func() {
		err := ShowUI(stateChan, mgr.Apps)
		if err != nil {
			fmt.Printf("Failed to show UI: %v\n", err)
		}
	}, func() {
		os.Exit(0)
	})
}
