package main

import (
	_ "embed"
	"fmt"
	"os"
)

//go:embed icon.ico
var iconData []byte

func main() {
	// Parse config
	cfg, err := LoadConfig("config.toml")
	if err != nil {
		fmt.Printf("Failed to load config.toml: %v\n", err)
		os.Exit(1)
	}

	stateChan := make(chan *AppInfo, 100)
	mgr := NewManager(cfg, stateChan)

	// Start monitoring apps that are enabled by default
	mgr.StartAll()

	// Dispatch state updates to the UI, if active
	go func() {
		for app := range stateChan {
			// Find index
			id := -1
			for i, a := range mgr.Apps {
				if a == app {
					id = i
					break
				}
			}
			if ui := GetCurrentUI(); ui != nil && id != -1 {
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
