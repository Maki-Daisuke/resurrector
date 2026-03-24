package main

import (
	"github.com/energye/systray"
)

// RunSystray starts the system tray loop and registers callbacks.
func RunSystray(iconData []byte, onOpenUI func(), onExit func()) {
	systray.Run(func() {
		if len(iconData) > 0 {
			systray.SetIcon(iconData)
		}
		systray.SetTitle("Resurrector")
		systray.SetTooltip("Resurrector Process Monitor")

		mOpen := systray.AddMenuItem("Open Settings", "Open UI")
		mQuit := systray.AddMenuItem("Quit", "Quit Resurrector")

		mOpen.Click(func() {
			onOpenUI()
		})
		mQuit.Click(func() {
			onExit()
			systray.Quit()
		})
	}, func() {
		// onExit
	})
}
