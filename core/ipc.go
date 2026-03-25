package main

import (
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"path/filepath"

	"resurrector/util"
)

type UIProcess struct {
	cmd    *exec.Cmd
	client *rpc.Client
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

	// Create JSON-RPC client over stdio
	conn := &util.StdioConn{ReadCloser: stdout, WriteCloser: stdin}
	client := jsonrpc.NewClient(conn)

	ui := &UIProcess{
		cmd:    cmd,
		client: client,
	}
	currentUI = ui

	// Send initial state
	for _, app := range apps {
		ui.SendState(app)
	}

	// Wait for UI to exit in background
	go func() {
		cmd.Wait()
		currentUI = nil
	}()

	return nil
}

func (ui *UIProcess) SendState(app *AppInfo) {
	if ui == nil || ui.client == nil {
		return
	}
	msg := map[string]interface{}{
		"name":         app.Config.Name,
		"pid":          app.PID,
		"state":        string(app.State),
		"enabled":      app.Config.Enabled,
		"command":      app.Config.Command,
		"restartCount": app.RestartCount,
	}

	// Send to UI as a notification using Go (asynchronous call)
	var reply bool
	ui.client.Go("UI.UpdateState", msg, &reply, nil)
}

func GetCurrentUI() *UIProcess {
	return currentUI
}
