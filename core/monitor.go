package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unsafe"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
)

type AppState string

const (
	StateStopped  AppState = "Stopped"
	StateRunning  AppState = "Running"
	StateRetrying AppState = "Retrying"
	StateFailed   AppState = "Failed" // Exhausted max retries
)

// AppInfo includes the static config and runtime state.
type AppInfo struct {
	Config       App
	State        AppState
	PID          int
	RestartCount int
	stopChan     chan struct{}
}

// Manager handles all monitored applications.
type Manager struct {
	Apps      []*AppInfo
	StateChan chan *AppInfo // Used to notify UI of state changes
}

func NewManager(cfg *Config, stateChan chan *AppInfo) *Manager {
	m := &Manager{
		StateChan: stateChan,
	}
	for _, appCfg := range cfg.Apps {
		app := &AppInfo{
			Config:   *appCfg,
			State:    StateStopped,
			stopChan: make(chan struct{}),
		}
		m.Apps = append(m.Apps, app)
	}
	sort.Slice(m.Apps, func(i, j int) bool {
		return m.Apps[i].Config.Name < m.Apps[j].Config.Name
	})
	return m
}

func (m *Manager) StartAll() {
	for _, app := range m.Apps {
		if app.Config.Enabled {
			go m.monitorLoop(app)
		}
	}
}

func (m *Manager) setState(app *AppInfo, state AppState) {
	app.State = state
	if m.StateChan != nil {
		m.StateChan <- app
	}
}

func (m *Manager) monitorLoop(app *AppInfo) {
	app.RestartCount = 0

	for {
		app.PID = 0
		startTime := time.Now()

		// Search for an existing process
		pid, err := findExistingProcess(app.Config)
		if err == nil && pid > 0 {
			// Found — attach and monitor
			app.PID = pid
			_ = monitorExistingProcess(pid, app.stopChan, func(p int) {
				m.setState(app, StateRunning)
			})
		} else {
			// Not found — start a new process and attach
			newPID, startErr := startProcess(app.Config)
			if startErr != nil {
				app.RestartCount++
				if app.Config.MaxRetries > 0 && app.RestartCount > app.Config.MaxRetries {
					m.setState(app, StateFailed)
					return
				}
				m.setState(app, StateRetrying)
				select {
				case <-time.After(time.Duration(app.Config.RestartDelaySec) * time.Second):
					continue
				case <-app.stopChan:
					m.setState(app, StateStopped)
					return
				}
			}
			app.PID = newPID
			_ = monitorExistingProcess(newPID, app.stopChan, func(p int) {
				m.setState(app, StateRunning)
			})
		}

		duration := time.Since(startTime)
		app.PID = 0

		// Process exited (or was stopped)

		select {
		case <-app.stopChan:
			m.setState(app, StateStopped)
			return
		default:
		}

		if duration >= time.Duration(app.Config.HealthyTimeoutSec)*time.Second {
			app.RestartCount = 0
		} else {
			app.RestartCount++
		}

		if app.Config.MaxRetries > 0 && app.RestartCount > app.Config.MaxRetries {
			m.setState(app, StateFailed)
			return
		}

		m.setState(app, StateRetrying)

		// Wait before restart
		select {
		case <-time.After(time.Duration(app.Config.RestartDelaySec) * time.Second):
			// continue loop
		case <-app.stopChan:
			m.setState(app, StateStopped)
			return
		}
	}
}

// startProcess creates a new detached process and returns its PID.
// The process is NOT tied to a Job Object, so it survives when resurrector exits.
func startProcess(cfg App) (int, error) {
	// Build Command Line
	args := []string{cfg.Command}
	args = append(args, cfg.Args...)

	var escapedArgs []string
	for _, arg := range args {
		escapedArgs = append(escapedArgs, windows.EscapeArg(arg))
	}
	cmdline := strings.Join(escapedArgs, " ")
	cmdlinePtr, err := windows.UTF16PtrFromString(cmdline)
	if err != nil {
		return 0, err
	}

	var dirPtr *uint16
	if cfg.CWD != "" {
		dirPtr, err = windows.UTF16PtrFromString(cfg.CWD)
		if err != nil {
			return 0, err
		}
	}

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	if cfg.HideWindow {
		si.Flags |= windows.STARTF_USESHOWWINDOW
		si.ShowWindow = windows.SW_HIDE
	}

	var pi windows.ProcessInformation

	var creationFlags uint32
	if cfg.HideWindow {
		creationFlags = windows.CREATE_NO_WINDOW
	} else {
		creationFlags = windows.CREATE_NEW_CONSOLE
	}

	err = windows.CreateProcess(
		nil,
		cmdlinePtr,
		nil,
		nil,
		false,
		creationFlags,
		nil,
		dirPtr,
		&si,
		&pi,
	)
	if err != nil {
		return 0, fmt.Errorf("CreateProcess: %w", err)
	}

	// ハンドルを即座に閉じてデタッチする
	windows.CloseHandle(pi.Thread)
	windows.CloseHandle(pi.Process)

	return int(pi.ProcessId), nil
}

func findExistingProcess(cfg App) (int, error) {
	procs, err := process.Processes()
	if err != nil {
		return 0, err
	}

	args := []string{cfg.Command}
	args = append(args, cfg.Args...)

	var escapedArgs []string
	for _, arg := range args {
		escapedArgs = append(escapedArgs, windows.EscapeArg(arg))
	}
	expectedCmdline := strings.Join(escapedArgs, " ")

	for _, p := range procs {
		cmdline, err := p.Cmdline()
		if err != nil {
			continue
		}

		if cmdline == expectedCmdline {
			return int(p.Pid), nil
		}

		slice, err := p.CmdlineSlice()
		if err != nil {
			continue
		}

		if len(slice) == len(args) {
			match := true
			base1 := filepath.Base(slice[0])
			base2 := filepath.Base(args[0])
			if !strings.EqualFold(base1, base2) {
				match = false
			} else {
				for i := 1; i < len(args); i++ {
					if slice[i] != args[i] {
						match = false
						break
					}
				}
			}
			if match {
				return int(p.Pid), nil
			}
		}
	}
	return 0, nil
}

// monitorExistingProcess monitors an already-running process by its PID.
// When stop is requested, it just stops monitoring without killing the process.
func monitorExistingProcess(pid int, stopChan chan struct{}, onStart func(int)) error {
	access := uint32(windows.SYNCHRONIZE | windows.PROCESS_QUERY_LIMITED_INFORMATION)
	handle, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("OpenProcess: %w", err)
	}
	defer windows.CloseHandle(handle)

	onStart(pid)

	stopEvent, _ := windows.CreateEvent(nil, 0, 0, nil)
	defer windows.CloseHandle(stopEvent)

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-stopChan:
			windows.SetEvent(stopEvent)
		case <-done:
			// Process exited naturally — clean up goroutine
		}
	}()

	handles := []windows.Handle{handle, stopEvent}

	for {
		event, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
		if err != nil {
			return err
		}

		switch event {
		case windows.WAIT_OBJECT_0: // Process exited
			return nil
		case windows.WAIT_OBJECT_0 + 1: // Stop requested — just detach, don't kill
			return nil
		}
	}
}
