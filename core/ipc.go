package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// UIProcess represents a running UI process and its JSON encoder.
type UIProcess struct {
	cmd     *exec.Cmd
	encoder *json.Encoder
}

var (
	currentUI   *UIProcess
	currentUIMu sync.Mutex
)

// ShowUI launches the UI process and sends the initial state from the reconciler.
func ShowUI(reconciler *Reconciler) error {
	currentUIMu.Lock()
	defer currentUIMu.Unlock()

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
	_ = stdout // Ignore stdout since we don't read JSON-RPC responses anymore
	cmd.Stderr = os.Stderr // pipe ui logs directly

	if err := cmd.Start(); err != nil {
		return err
	}

	encoder := json.NewEncoder(stdin)

	ui := &UIProcess{
		cmd:     cmd,
		encoder: encoder,
	}
	currentUI = ui

	// Send initial state from the reconciler
	statuses := reconciler.AllStatuses()
	for _, status := range statuses {
		ui.SendState(status)
	}

	// Wait for UI to exit in background
	go func() {
		cmd.Wait()
		currentUIMu.Lock()
		currentUI = nil
		currentUIMu.Unlock()
	}()

	return nil
}

// SendState sends a state update to the UI process via JSON-RPC.
func (ui *UIProcess) SendState(status MonitorStatus) {
	if ui == nil || ui.encoder == nil {
		return
	}
	msg := map[string]interface{}{
		"name":         status.Name,
		"pid":          status.PID,
		"state":        string(status.State),
		"enabled":      status.Enabled,
		"command":      status.Command,
		"args":         status.Args,
		"restartCount": status.RestartCount,
	}

	log.Printf("[IPC] Sending UI.UpdateState: %q -> %s", status.Name, status.State)
	ui.encoder.Encode(msg)
}

// GetCurrentUI returns the current UI process, or nil if not running.
func GetCurrentUI() *UIProcess {
	currentUIMu.Lock()
	defer currentUIMu.Unlock()
	return currentUI
}
