package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unsafe"

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
		m.setState(app, StateRunning)

		startTime := time.Now()
		_ = runProcess(app.Config, app.stopChan)
		duration := time.Since(startTime)

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

func runProcess(cfg App, stopChan chan struct{}) error {
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
		return err
	}

	var dirPtr *uint16
	if cfg.CWD != "" {
		dirPtr, err = windows.UTF16PtrFromString(cfg.CWD)
		if err != nil {
			return err
		}
	}

	// Create Job Object
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("CreateJobObject: %w", err)
	}
	defer windows.CloseHandle(job)

	// Set JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE

	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		return fmt.Errorf("SetInformationJobObject: %w", err)
	}

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	if cfg.HideWindow {
		si.Flags |= windows.STARTF_USESHOWWINDOW
		si.ShowWindow = windows.SW_HIDE
	}

	var pi windows.ProcessInformation

	creationFlags := uint32(windows.CREATE_SUSPENDED)
	if cfg.HideWindow {
		creationFlags |= windows.CREATE_NO_WINDOW
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
		return fmt.Errorf("CreateProcess: %w", err)
	}

	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)

	// Assign to Job
	err = windows.AssignProcessToJobObject(job, pi.Process)
	if err != nil {
		windows.TerminateProcess(pi.Process, 1)
		return fmt.Errorf("AssignProcessToJobObject: %w", err)
	}

	// Resume thread
	_, err = windows.ResumeThread(pi.Thread)
	if err != nil {
		windows.TerminateProcess(pi.Process, 1)
		return fmt.Errorf("ResumeThread: %w", err)
	}

	// Monitor loop
	// Create an event for stopChan so we can wait on multiple objects
	stopEvent, _ := windows.CreateEvent(nil, 0, 0, nil)
	defer windows.CloseHandle(stopEvent)

	go func() {
		<-stopChan
		windows.SetEvent(stopEvent)
	}()

	handles := []windows.Handle{pi.Process, stopEvent}

	for {
		event, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
		if err != nil {
			return err
		}

		switch event {
		case windows.WAIT_OBJECT_0: // Process exited
			return nil
		case windows.WAIT_OBJECT_0 + 1: // Stop requested
			// Closing the job handle (via defer) kills the process
			return nil
		}
	}
}
