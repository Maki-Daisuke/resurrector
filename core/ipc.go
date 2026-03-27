package main

import (
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"resurrector/util"
)

// UIProcess represents a running UI process and its RPC client.
type UIProcess struct {
	cmd    *exec.Cmd
	client interface {
		Go(serviceMethod string, args interface{}, reply interface{}, done chan *rpc.Call) *rpc.Call
	}
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
	if ui == nil || ui.client == nil {
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

	// Send to UI as a notification using Go (asynchronous call)
	var reply bool
	ui.client.Go("UI.UpdateState", msg, &reply, nil)
}

// GetCurrentUI returns the current UI process, or nil if not running.
func GetCurrentUI() *UIProcess {
	currentUIMu.Lock()
	defer currentUIMu.Unlock()
	return currentUI
}
