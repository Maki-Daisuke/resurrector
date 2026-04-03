package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"resurrector/util"
)

// AppState represents the lifecycle state of a monitored process.
type AppState string

const (
	StateStopped  AppState = "Stopped"
	StateStarting AppState = "Starting"
	StateRunning  AppState = "Running"
	StateRetrying AppState = "Retrying"
	StateFailed   AppState = "Failed"
	StateRemoved  AppState = "Removed"
)

// MonitorStatus is the snapshot of a Monitor's current state, sent to the UI via IPC.
type MonitorStatus struct {
	Name         string   `json:"name"`
	State        AppState `json:"state"`
	PID          int      `json:"pid"`
	Enabled      bool     `json:"enabled"`
	Command      string   `json:"command"`
	Args         []string `json:"args"`
	RestartCount int      `json:"restartCount"`
}

// Monitor manages the lifecycle of a single monitored process.
// It runs a monitoring goroutine that handles starting, watching, restarting,
// and stopping the process using a Windows Job Object.
type Monitor struct {
	mu sync.Mutex

	config util.App
	state  AppState
	pid    int

	restartCount int

	// Job Object handle for the current process tree.
	jobHandle windows.Handle

	// stopChan signals the monitor goroutine to stop.
	stopChan chan struct{}
	// doneChan is closed when the monitor goroutine has fully exited.
	doneChan chan struct{}

	// onStateChange is called whenever the monitor's state changes.
	onStateChange func(MonitorStatus)
}

// NewMonitor creates a new Monitor for the given config entry.
func NewMonitor(cfg util.App, onStateChange func(MonitorStatus)) *Monitor {
	return &Monitor{
		config:        cfg,
		state:         StateStopped,
		onStateChange: onStateChange,
	}
}

// Status returns a snapshot of the monitor's current state.
func (m *Monitor) Status() MonitorStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return MonitorStatus{
		Name:         m.config.Name,
		State:        m.state,
		PID:          m.pid,
		Enabled:      m.config.Enabled,
		Command:      m.config.Command,
		Args:         m.config.Args,
		RestartCount: m.restartCount,
	}
}

// Start launches the monitoring goroutine. If already running, it is a no-op.
func (m *Monitor) Start() {
	m.mu.Lock()
	if m.stopChan != nil {
		m.mu.Unlock()
		return // already running
	}
	m.stopChan = make(chan struct{})
	m.doneChan = make(chan struct{})
	m.mu.Unlock()

	go m.monitorLoop()
}

// Stop signals the monitoring goroutine to stop and waits for it to finish.
// The Job Object handle is closed, which kills the entire process tree.
func (m *Monitor) Stop() {
	m.mu.Lock()
	if m.stopChan == nil {
		m.mu.Unlock()
		return // not running
	}
	close(m.stopChan)
	m.stopChan = nil
	doneChan := m.doneChan
	m.mu.Unlock()

	// Wait for the goroutine to finish
	<-doneChan
}

// UpdateMonitoringParams hot-reloads monitoring parameters without restarting the process.
func (m *Monitor) UpdateMonitoringParams(cfg util.App) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.RestartDelaySec = cfg.RestartDelaySec
	m.config.HealthyTimeoutSec = cfg.HealthyTimeoutSec
	m.config.MaxRetries = cfg.MaxRetries
}

// SetConfig replaces the monitor's config. This is used by the Reconciler
// to update fields (e.g. Enabled) before calling Stop(), so that any
// state change notifications emitted during shutdown reflect the correct config.
func (m *Monitor) SetConfig(cfg util.App) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = cfg
}

// Config returns the current config for this monitor.
func (m *Monitor) Config() util.App {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config
}

// setState updates the state and notifies the callback.
func (m *Monitor) setState(state AppState) {
	m.state = state
	if m.onStateChange != nil {
		m.onStateChange(m.statusLocked())
	}
}

// statusLocked returns a status snapshot. Must be called with m.mu held.
func (m *Monitor) statusLocked() MonitorStatus {
	return MonitorStatus{
		Name:         m.config.Name,
		State:        m.state,
		PID:          m.pid,
		Enabled:      m.config.Enabled,
		Command:      m.config.Command,
		Args:         m.config.Args,
		RestartCount: m.restartCount,
	}
}

// monitorLoop is the main goroutine that manages the process lifecycle.
func (m *Monitor) monitorLoop() {
	defer func() {
		m.mu.Lock()
		m.stopChan = nil
		m.doneChan = nil
		m.mu.Unlock()
	}()
	defer close(m.doneChan)

	m.mu.Lock()
	m.restartCount = 0
	stopChan := m.stopChan
	m.mu.Unlock()

	for {
		m.mu.Lock()
		m.pid = 0
		m.setState(StateStarting)
		cfg := m.config // snapshot under lock
		m.mu.Unlock()

		startTime := time.Now()

		pid, jobHandle, err := startProcessWithJobObject(cfg)
		if err != nil {
			log.Printf("[%s] Failed to start process: %v", cfg.Name, err)

			m.mu.Lock()
			m.restartCount++
			if cfg.MaxRetries >= 0 && m.restartCount > cfg.MaxRetries {
				m.setState(StateFailed)
				m.mu.Unlock()
				return
			}
			m.setState(StateRetrying)
			delay := time.Duration(m.config.RestartDelaySec) * time.Second
			m.mu.Unlock()

			select {
			case <-time.After(delay):
				continue
			case <-stopChan:
				m.mu.Lock()
				m.setState(StateStopped)
				m.mu.Unlock()
				return
			}
		}

		m.mu.Lock()
		m.pid = pid
		m.jobHandle = jobHandle
		m.setState(StateRunning)
		m.mu.Unlock()

		// Wait for the process to exit or stop signal
		exitedNaturally := waitForProcessOrStop(pid, stopChan)

		m.mu.Lock()
		m.pid = 0
		// Close the Job Object handle to clean up the process tree
		if m.jobHandle != 0 {
			windows.CloseHandle(m.jobHandle)
			m.jobHandle = 0
		}
		m.mu.Unlock()

		if !exitedNaturally {
			// Stop was requested
			m.mu.Lock()
			m.setState(StateStopped)
			m.mu.Unlock()
			return
		}

		// Process exited naturally — determine if it was healthy
		m.mu.Lock()
		duration := time.Since(startTime)
		healthyTimeout := time.Duration(m.config.HealthyTimeoutSec) * time.Second

		if duration >= healthyTimeout {
			m.restartCount = 0
		} else {
			m.restartCount++
		}

		if m.config.MaxRetries >= 0 && m.restartCount > m.config.MaxRetries {
			m.setState(StateFailed)
			m.mu.Unlock()
			return
		}

		m.setState(StateRetrying)
		delay := time.Duration(m.config.RestartDelaySec) * time.Second
		m.mu.Unlock()

		select {
		case <-time.After(delay):
			// continue restart loop
		case <-stopChan:
			m.mu.Lock()
			m.setState(StateStopped)
			m.mu.Unlock()
			return
		}
	}
}

// startProcessWithJobObject creates a Windows Job Object, spawns the process,
// and assigns it to the Job Object. The Job Object is configured with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE so that closing the handle kills the
// entire process tree.
func startProcessWithJobObject(cfg util.App) (pid int, jobHandle windows.Handle, err error) {
	// Create Job Object
	jobHandle, err = windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("CreateJobObject: %w", err)
	}

	// Configure Job Object to kill all processes when the handle is closed
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		jobHandle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		windows.CloseHandle(jobHandle)
		return 0, 0, fmt.Errorf("SetInformationJobObject: %w", err)
	}

	// Build command line
	args := []string{cfg.Command}
	args = append(args, cfg.Args...)

	var escapedArgs []string
	for _, arg := range args {
		escapedArgs = append(escapedArgs, windows.EscapeArg(arg))
	}
	cmdline := strings.Join(escapedArgs, " ")
	cmdlinePtr, err := windows.UTF16PtrFromString(cmdline)
	if err != nil {
		windows.CloseHandle(jobHandle)
		return 0, 0, fmt.Errorf("UTF16PtrFromString(cmdline): %w", err)
	}

	var dirPtr *uint16
	if cfg.CWD != "" {
		dirPtr, err = windows.UTF16PtrFromString(cfg.CWD)
		if err != nil {
			windows.CloseHandle(jobHandle)
			return 0, 0, fmt.Errorf("UTF16PtrFromString(cwd): %w", err)
		}
	}

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	if cfg.HideWindow {
		si.Flags |= windows.STARTF_USESHOWWINDOW
		si.ShowWindow = windows.SW_HIDE
	}

	var creationFlags uint32 = windows.CREATE_SUSPENDED
	if cfg.HideWindow {
		creationFlags |= windows.CREATE_NO_WINDOW
	} else {
		creationFlags |= windows.CREATE_NEW_CONSOLE
	}

	var pi windows.ProcessInformation
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
		windows.CloseHandle(jobHandle)
		return 0, 0, fmt.Errorf("CreateProcess: %w", err)
	}

	// Assign the process to the Job Object before resuming it.
	// This ensures child processes spawned after resume are also captured.
	err = windows.AssignProcessToJobObject(jobHandle, pi.Process)
	if err != nil {
		// Kill the suspended process and clean up
		windows.TerminateProcess(pi.Process, 1)
		windows.CloseHandle(pi.Thread)
		windows.CloseHandle(pi.Process)
		windows.CloseHandle(jobHandle)
		return 0, 0, fmt.Errorf("AssignProcessToJobObject: %w", err)
	}

	// Resume the process now that it's inside the Job Object
	ret, err := windows.ResumeThread(pi.Thread)
	if ret == 0xFFFFFFFF || err != nil {
		windows.TerminateProcess(pi.Process, 1)
		windows.CloseHandle(pi.Thread)
		windows.CloseHandle(pi.Process)
		windows.CloseHandle(jobHandle)
		return 0, 0, fmt.Errorf("ResumeThread: %w", err)
	}

	windows.CloseHandle(pi.Thread)
	windows.CloseHandle(pi.Process)

	log.Printf("[%s] Process started (PID: %d, Job Object: assigned)", cfg.Name, pi.ProcessId)
	return int(pi.ProcessId), jobHandle, nil
}

// waitForProcessOrStop waits for a process to exit or for a stop signal.
// Returns true if the process exited naturally, false if stop was requested.
func waitForProcessOrStop(pid int, stopChan <-chan struct{}) bool {
	processHandle, err := windows.OpenProcess(
		windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(pid),
	)
	if err != nil {
		// Process already gone
		return true
	}
	defer windows.CloseHandle(processHandle)

	stopEvent, _ := windows.CreateEvent(nil, 0, 0, nil)
	defer windows.CloseHandle(stopEvent)

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-stopChan:
			windows.SetEvent(stopEvent)
		case <-done:
		}
	}()

	handles := []windows.Handle{processHandle, stopEvent}

	event, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
	if err != nil {
		return true // assume exited on error
	}

	switch event {
	case windows.WAIT_OBJECT_0: // Process exited
		return true
	case windows.WAIT_OBJECT_0 + 1: // Stop requested
		return false
	default:
		return true
	}
}
