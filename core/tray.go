package main

import (
	"fmt"
	"log/slog"

	"github.com/energye/systray"
)

// RunSystray starts the system tray loop and registers callbacks.
func RunSystray(iconData []byte, autoStart *AutoStartManager, onOpenUI func(), onOpenConfigWith func(), onExit func()) {
	systray.Run(func() {
		if len(iconData) > 0 {
			systray.SetIcon(iconData)
		}
		systray.SetTitle("Resurrector")
		systray.SetTooltip("Resurrector Process Monitor")

		mOpen := systray.AddMenuItem("Open Settings", "Open UI")
		mOpenConfigWith := systray.AddMenuItem("Open config with...", "Choose an app to open config.toml")
		var mAutoStart *systray.MenuItem
		if autoStart != nil {
			mAutoStart = systray.AddMenuItem("Auto-start Resurrector", "Start Resurrector automatically when you sign in to Windows")
			enabled, err := autoStart.Enabled()
			if err != nil {
				slog.Warn("failed to read logon startup setting",
					slog.String("component", "tray"),
					slog.Any("error", err),
				)
			} else if enabled {
				mAutoStart.Check()
			}
			systray.AddSeparator()
		}
		mQuit := systray.AddMenuItem("Quit", "Quit Resurrector")

		mOpen.Click(func() {
			onOpenUI()
		})
		mOpenConfigWith.Click(func() {
			onOpenConfigWith()
		})
		if mAutoStart != nil {
			mAutoStart.Click(func() {
				nextEnabled := !mAutoStart.Checked()
				if err := autoStart.SetEnabled(nextEnabled); err != nil {
					slog.Error("failed to update logon startup setting",
						slog.String("component", "tray"),
						slog.Bool("enabled", nextEnabled),
						slog.Any("error", err),
					)
					showErrorDialog("Resurrector - Error", fmt.Sprintf("Failed to update the logon startup setting:\n\n%v", err))
					return
				}

				enabled, err := autoStart.Enabled()
				if err != nil {
					slog.Error("failed to refresh logon startup setting",
						slog.String("component", "tray"),
						slog.Any("error", err),
					)
					showErrorDialog("Resurrector - Error", fmt.Sprintf("The logon startup setting was updated, but its final state could not be confirmed:\n\n%v", err))
					return
				}

				if enabled {
					mAutoStart.Check()
					return
				}
				mAutoStart.Uncheck()
			})
		}
		mQuit.Click(func() {
			onExit()
			systray.Quit()
		})
	}, func() {
		// onExit
	})
}
