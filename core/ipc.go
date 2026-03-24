package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// UICommand represents a command from the UI.
type UICommand struct {
	Action string `json:"action"` // e.g. "exit", "stop"
	AppID  int    `json:"app_id"`
}

type UIProcess struct {
	cmd       *exec.Cmd
	stdinPipe io.WriteCloser
	stateChan chan *AppInfo
}

var currentUI *UIProcess

// ShowUI launches the UI process and bridges communication.
func ShowUI(stateChan chan *AppInfo, apps []*AppInfo) error {
	if currentUI != nil {
		return nil // Already running
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	uiExe := filepath.Join(filepath.Dir(exePath), "resurrector-ui.exe")

	cmd := exec.Command(uiExe)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr // pipe ui logs directly

	if err := cmd.Start(); err != nil {
		return err
	}

	ui := &UIProcess{
		cmd:       cmd,
		stdinPipe: stdin,
		stateChan: stateChan,
	}
	currentUI = ui

	// Send initial state
	for i, app := range apps {
		ui.SendState(i, app)
	}

	// Read commands from UI
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var uiCmd UICommand
			if err := json.Unmarshal(scanner.Bytes(), &uiCmd); err == nil {
				// We can handle incoming UI commands here if needed later
			}
		}
		currentUI = nil
	}()

	return nil
}

func (ui *UIProcess) SendState(id int, app *AppInfo) {
	if ui == nil || ui.stdinPipe == nil {
		return
	}
	msg := map[string]interface{}{
		"id":           id,
		"name":         app.Config.Name,
		"state":        string(app.State),
		"restartCount": app.RestartCount,
	}
	b, _ := json.Marshal(msg)
	b = append(b, '\n')
	ui.stdinPipe.Write(b)
}

func GetCurrentUI() *UIProcess {
	return currentUI
}
