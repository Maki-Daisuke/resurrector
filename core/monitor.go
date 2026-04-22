package main

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
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

	// Process handle for the current root process.
	processHandle windows.Handle
	// Job Object handle for the current process tree.
	jobHandle windows.Handle

	// stopChan signals the monitor goroutine to stop.
	stopChan chan struct{}
	// doneChan is closed when the monitor goroutine has fully exited.
	doneChan chan struct{}

	// onStateChange is called whenever the monitor's state changes.
	onStateChange func(MonitorStatus)
}

var (
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetClassName             = user32.NewProc("GetClassNameW")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procPostMessage              = user32.NewProc("PostMessageW")
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole            = kernel32.NewProc("AttachConsole")
	procFreeConsole              = kernel32.NewProc("FreeConsole")
	procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
)

const (
	wmClose            = 0x0010
	consoleWindowClass = "ConsoleWindowClass"
	// Windows 10+ with ConPTY presents console windows under this class
	// instead of ConsoleWindowClass.
	pseudoConsoleWindowClass = "PseudoConsoleWindow"
)

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
// The monitor applies graceful stop when configured, then falls back to killing
// the entire process tree via the Job Object.
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

	<-doneChan
}

// UpdateMonitoringParams hot-reloads monitoring parameters without restarting the process.
func (m *Monitor) UpdateMonitoringParams(cfg util.App) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.RestartDelaySec = cfg.RestartDelaySec
	m.config.HealthyTimeoutSec = cfg.HealthyTimeoutSec
	m.config.MaxRetries = cfg.MaxRetries
	m.config.StopCommand = cfg.StopCommand
	m.config.StopArgs = append([]string{}, cfg.StopArgs...)
	m.config.StopTimeoutSec = cfg.StopTimeoutSec
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

		pid, processHandle, jobHandle, err := startProcessWithJobObject(cfg)
		if err != nil {
			slog.Error("failed to start process",
				slog.String("component", "monitor"),
				slog.String("app", cfg.Name),
				slog.Any("error", err),
			)

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
		m.processHandle = processHandle
		m.jobHandle = jobHandle
		m.setState(StateRunning)
		m.mu.Unlock()

		// Wait for the process to exit or stop signal
		exitedNaturally := waitForProcessOrStop(processHandle, stopChan)

		if !exitedNaturally {
			// Stop was requested
			m.mu.Lock()
			cfg := m.config
			activePID := m.pid
			processHandle := m.processHandle
			jobHandle := m.jobHandle
			m.mu.Unlock()

			if err := stopProcess(cfg, activePID, processHandle); err != nil {
				slog.Warn("stop strategy failed, continuing cleanup",
					slog.String("component", "monitor"),
					slog.String("app", cfg.Name),
					slog.Any("error", err),
				)
			}

			m.mu.Lock()
			m.pid = 0
			m.closeHandlesLocked(jobHandle)
			m.setState(StateStopped)
			m.mu.Unlock()
			return
		}

		// Process exited naturally — determine if it was healthy
		m.mu.Lock()
		m.pid = 0
		m.closeHandlesLocked(m.jobHandle)
		duration := time.Since(startTime)
		healthyTimeout := time.Duration(m.config.HealthyTimeoutSec) * time.Second

		// healthy_timeout_sec = 0 means "do not reset the counter on uptime" — the
		// counter always increments on exit so max_retries remains meaningful.
		if healthyTimeout > 0 && duration >= healthyTimeout {
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

func (m *Monitor) closeHandlesLocked(jobHandle windows.Handle) {
	if m.processHandle != 0 {
		windows.CloseHandle(m.processHandle)
		m.processHandle = 0
	}
	if jobHandle != 0 {
		windows.CloseHandle(jobHandle)
	}
	m.jobHandle = 0
}

// startProcessWithJobObject creates a Windows Job Object, spawns the process,
// and assigns it to the Job Object. The Job Object is configured with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE so that closing the handle kills the
// entire process tree.
func startProcessWithJobObject(cfg util.App) (pid int, processHandle windows.Handle, jobHandle windows.Handle, err error) {
	jobHandle, err = windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("CreateJobObject: %w", err)
	}

	// Configure the Job Object to kill all processes when the handle is closed.
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
		return 0, 0, 0, fmt.Errorf("SetInformationJobObject: %w", err)
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
		return 0, 0, 0, fmt.Errorf("UTF16PtrFromString(cmdline): %w", err)
	}

	var dirPtr *uint16
	if cfg.CWD != "" {
		dirPtr, err = windows.UTF16PtrFromString(cfg.CWD)
		if err != nil {
			windows.CloseHandle(jobHandle)
			return 0, 0, 0, fmt.Errorf("UTF16PtrFromString(cwd): %w", err)
		}
	}

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	if cfg.HideWindow {
		si.Flags |= windows.STARTF_USESHOWWINDOW
		si.ShowWindow = windows.SW_HIDE
	}

	creationFlags := processCreationFlags()

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
		return 0, 0, 0, fmt.Errorf("CreateProcess: %w", err)
	}

	// Assign the process to the Job Object before resuming it, so that any
	// children spawned after resume are also captured by the job.
	err = windows.AssignProcessToJobObject(jobHandle, pi.Process)
	if err != nil {
		windows.TerminateProcess(pi.Process, 1)
		windows.CloseHandle(pi.Thread)
		windows.CloseHandle(pi.Process)
		windows.CloseHandle(jobHandle)
		return 0, 0, 0, fmt.Errorf("AssignProcessToJobObject: %w", err)
	}

	ret, err := windows.ResumeThread(pi.Thread)
	if ret == 0xFFFFFFFF || err != nil {
		windows.TerminateProcess(pi.Process, 1)
		windows.CloseHandle(pi.Thread)
		windows.CloseHandle(pi.Process)
		windows.CloseHandle(jobHandle)
		return 0, 0, 0, fmt.Errorf("ResumeThread: %w", err)
	}

	windows.CloseHandle(pi.Thread)

	slog.Info("process started",
		slog.String("component", "monitor"),
		slog.String("app", cfg.Name),
		slog.Uint64("pid", uint64(pi.ProcessId)),
	)
	return int(pi.ProcessId), pi.Process, jobHandle, nil
}

func processCreationFlags() uint32 {
	return uint32(windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP)
}

// waitForProcessOrStop waits for a process to exit or for a stop signal.
// Returns true if the process exited naturally, false if stop was requested.
func waitForProcessOrStop(processHandle windows.Handle, stopChan <-chan struct{}) bool {
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

func stopProcess(cfg util.App, pid int, processHandle windows.Handle) error {
	timeout := time.Duration(cfg.StopTimeoutSec) * time.Second
	if cfg.StopCommand != "" {
		return stopWithCommand(cfg, pid, processHandle, timeout)
	}
	return stopAutomatically(cfg, pid, processHandle, timeout)
}

func stopWithCommand(cfg util.App, pid int, processHandle windows.Handle, timeout time.Duration) error {
	if err := launchStopCommand(cfg, pid); err != nil {
		slog.Warn("stop_command failed to start, falling back to TerminateProcess",
			slog.String("component", "monitor"),
			slog.String("app", cfg.Name),
			slog.Any("error", err),
		)
		return terminateProcess(processHandle)
	}
	return waitForExitOrTerminate(processHandle, timeout, "stop_command")
}

func launchStopCommand(cfg util.App, pid int) error {
	if cfg.StopCommand == "" {
		return nil
	}
	args := expandStopArgs(cfg.StopArgs, pid)

	cmd := exec.Command(cfg.StopCommand, args...)
	if cfg.CWD != "" {
		cmd.Dir = cfg.CWD
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start stop_command: %w", err)
	}

	go func() {
		_ = cmd.Wait()
	}()

	return nil
}

func expandStopArgs(stopArgs []string, pid int) []string {
	if len(stopArgs) == 0 {
		return nil
	}

	pidText := strconv.Itoa(pid)
	expanded := make([]string, len(stopArgs))
	for i, arg := range stopArgs {
		expanded[i] = strings.ReplaceAll(arg, "{pid}", pidText)
	}
	return expanded
}

func stopAutomatically(cfg util.App, pid int, processHandle windows.Handle, timeout time.Duration) error {
	if postCloseToProcessWindows(pid) > 0 {
		return waitForExitOrTerminate(processHandle, timeout, "automatic window stop")
	}

	cleanup, err := sendCtrlBreak(pid)
	if err != nil {
		slog.Warn("automatic console stop failed, falling back to TerminateProcess",
			slog.String("component", "monitor"),
			slog.String("app", cfg.Name),
			slog.Any("error", err),
		)
		return terminateProcess(processHandle)
	}
	defer cleanup()

	return waitForExitOrTerminate(processHandle, timeout, "automatic console stop")
}

func waitForExitOrTerminate(processHandle windows.Handle, timeout time.Duration, stopKind string) error {
	exited, err := waitForProcessExit(processHandle, timeout)
	if err != nil {
		return fmt.Errorf("waiting for %s: %w", stopKind, err)
	}
	if exited {
		return nil
	}
	return terminateProcess(processHandle)
}

func terminateProcess(processHandle windows.Handle) error {
	if err := windows.TerminateProcess(processHandle, 1); err != nil && err != windows.ERROR_ACCESS_DENIED {
		return fmt.Errorf("TerminateProcess: %w", err)
	}

	_, err := waitForProcessExit(processHandle, -1)
	return err
}

func waitForProcessExit(processHandle windows.Handle, timeout time.Duration) (bool, error) {
	waitMillis := uint32(windows.INFINITE)
	switch {
	case timeout == 0:
		waitMillis = 0
	case timeout > 0:
		waitMillis = uint32(timeout / time.Millisecond)
	}

	event, err := windows.WaitForSingleObject(processHandle, waitMillis)
	if err != nil {
		return false, err
	}

	switch event {
	case windows.WAIT_OBJECT_0:
		return true, nil
	case uint32(windows.WAIT_TIMEOUT):
		return false, nil
	default:
		slog.Warn("WaitForSingleObject returned unexpected status",
			slog.String("component", "monitor"),
			slog.Uint64("event", uint64(event)),
		)
		return false, nil
	}
}

func postCloseToProcessWindows(pid int) int {
	hwnds := topLevelWindowsForPID(pid)
	for _, hwnd := range hwnds {
		procPostMessage.Call(uintptr(hwnd), uintptr(wmClose), 0, 0)
	}
	return len(hwnds)
}

func topLevelWindowsForPID(pid int) []windows.Handle {
	hwnds := make([]windows.Handle, 0, 4)

	cb := syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		var windowPID uint32
		procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
		if int(windowPID) == pid {
			className, err := windowClassName(windows.Handle(hwnd))
			if err != nil {
				// Console hosts always expose a known class name, so a
				// read failure implies this is not a console window.
				// Treat it as a regular GUI window.
				slog.Warn("treating window with unreadable class as GUI",
					slog.String("component", "monitor"),
					slog.Int("pid", pid),
					slog.Any("error", err),
				)
				hwnds = append(hwnds, windows.Handle(hwnd))
				return 1
			}
			if !isWindowStopTargetClass(className) {
				return 1
			}
			hwnds = append(hwnds, windows.Handle(hwnd))
		}
		return 1
	})

	procEnumWindows.Call(cb, 0)
	return hwnds
}

func windowClassName(hwnd windows.Handle) (string, error) {
	buf := make([]uint16, 256)
	ret, _, err := procGetClassName.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if ret == 0 {
		return "", err
	}
	return windows.UTF16ToString(buf[:ret]), nil
}

func isWindowStopTargetClass(className string) bool {
	switch className {
	case consoleWindowClass, pseudoConsoleWindowClass:
		return false
	}
	return true
}

// consoleAttachMu serializes console attach/detach across the package. A
// process can only be attached to one console at a time, so concurrent
// sendCtrlBreak calls would otherwise race for the shared per-process
// console state.
var consoleAttachMu sync.Mutex

// sendCtrlBreak attaches to the target child's console and posts
// CTRL_BREAK_EVENT to the child's process group. The returned cleanup
// function MUST be called only after the child has exited (or the caller's
// timeout has elapsed); detaching from the console earlier can cause the
// event to be lost.
func sendCtrlBreak(pid int) (func(), error) {
	consoleAttachMu.Lock()

	// A process can only be attached to one console at a time.
	freeConsole()

	if err := attachConsole(uint32(pid)); err != nil {
		consoleAttachMu.Unlock()
		return nil, fmt.Errorf("AttachConsole: %w", err)
	}

	if err := generateConsoleCtrlEvent(uint32(pid)); err != nil {
		freeConsole()
		consoleAttachMu.Unlock()
		return nil, fmt.Errorf("GenerateConsoleCtrlEvent: %w", err)
	}

	return func() {
		freeConsole()
		consoleAttachMu.Unlock()
	}, nil
}

func attachConsole(pid uint32) error {
	r1, _, err := procAttachConsole.Call(uintptr(pid))
	if r1 == 0 {
		return err
	}
	return nil
}

func freeConsole() {
	procFreeConsole.Call()
}

func generateConsoleCtrlEvent(pid uint32) error {
	// The child was spawned with CREATE_NEW_PROCESS_GROUP, so its group ID
	// equals its PID. Targeting this specific group (rather than 0) confines
	// the event to the child and its descendants.
	r1, _, err := procGenerateConsoleCtrlEvent.Call(uintptr(windows.CTRL_BREAK_EVENT), uintptr(pid))
	if r1 == 0 {
		return err
	}
	return nil
}
