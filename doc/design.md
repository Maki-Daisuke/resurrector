# Resurrector Design & Architecture

This document provides detailed information about the internal design, architecture, and development environment of Resurrector.

## Design Philosophy

> **config.toml is the Single Source of Truth (SSoT).**

Resurrector treats `config.toml` as a **declarative desired-state specification** — similar to how Kubernetes treats manifests, or how Terraform treats `.tf` files. The running system continuously observes the config file and **reconciles** the actual process state to match the desired state described in it.

### Key Principles

1. **Declarative over Imperative**: Users declare _what_ should be running, not _how_ to transition. The core reconciles the difference automatically.
2. **Idempotent Reconciliation**: Applying the same config file repeatedly produces the same result. The reconciler compares desired state vs. actual state and performs only the necessary start/stop/restart actions to converge.
3. **File-Watch Driven**: The core uses `fsnotify` to detect config file changes in real-time. No restart of the core process is required to pick up configuration changes.
4. **Core is Read-Only After Bootstrap**: After startup initialization, the core process treats `config.toml` as read-only. Its only write is first-run bootstrap when the config file does not yet exist; after that it only reads and reacts. All ongoing config modifications are performed externally — either by the user editing the file directly or by the UI process writing to it.
5. **Strict Lifecycle Ownership**: Resurrector entirely owns the lifecycle of the monitored processes. It launches them as child processes bound to a **Windows Job Object**. If Resurrector exits, all monitored processes are guaranteed to terminate with it.

## System Overview

```text
+------------------------------------------------------------------+
|  Interactive Session (Windows Desktop)                           |
|                                                                  |
|            ~/.config/resurrector/config.toml                     |
|                  (Single Source of Truth)                         |
|                    ^                ^                             |
|         (fsnotify) |                | (direct write)             |
|           (read)   |                |                            |
|  +-----------------+---+         +--+----------------+           |
|  |   Core Process      |  (IPC)  |    UI Process     |          |
|  | (resurrector.exe)   |-------->| (resurrector-ui)  |          |
|  | [Read-Only config]  |  stdio  | [Wails + Svelte]  |          |
|  +---------+-----------+  state  +-------------------+          |
|            |              push                                   |
|            | (Reconcile / Monitor / Restart)                     |
|            v                                                     |
|  +-------------------+                                           |
|  | Monitored Process |                                           |
|  | (e.g. PowerToys)  |                                           |
|  +-------------------+                                           |
|                                                                  |
+------------------------------------------------------------------+
```

## Architecture

To minimize system resource consumption, Resurrector consists of two independent binaries: a **"resident core process"** and a **"disposable UI process."**

### 1. Core Process (`resurrector.exe`)

- **Role**: Steady presence in the system tray. Watches `config.toml` for changes (read-only) and reconciles monitored processes to match the desired state. By default it reads from `~/.config/resurrector/config.toml`, but accepts CLI flags such as `-f <path>`, `-log-file <path>`, and `-log-format <text|json>`.
- **Technology**: Go (Pure), `energye/systray`, `golang.org/x/sys/windows`, `github.com/fsnotify/fsnotify`
- **Features**: Does not have a UI; it continues to operate extremely lightly. When "Open Settings" is clicked from the system tray, it launches the UI process as a child process and passes runtime flags (`-f`, `-log-file`, `-log-format`) to keep behavior consistent. On first launch, if `config.toml` does not exist yet, it bootstraps the file with sample content.
- **Process Management**: Uses Windows Job Objects (`CreateJobObject`, `AssignProcessToJobObject`) to bind spawned child processes to its own lifecycle. This ensures that any command (even ones that spawn further sub-processes like `npm run start`) and its entire process tree are cleanly terminated when the core requests a stop or when the core process itself exits.
- **Process Lifetime Rules**: A session-local named mutex allows only one core instance per Windows session. A second launch shows an error dialog and exits without starting monitoring or a second tray icon.
- **Windows Integration**: The tray menu can toggle logon auto-start for the current user by writing to `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.

### 2. UI Process (`resurrector-ui.exe`)

- **Role**: Configuration screen for the user, real-time display of monitoring status. Provides a management interface to Create, Read, Update, and Delete (CRUD) entries in `config.toml` via a Wails bridge.
- **Technology**: Go + Wails + Svelte (TypeScript)
- **Features**: Uses a custom Wails logger. By default logs go to `STDERR`; the core then uses the UI process's `STDIN` as a unidirectional line-based JSON status stream. When `-log-file` is specified, logs are appended to that file instead. The UI **writes directly to `config.toml`** (using atomic writes) when the user makes configuration changes — the core detects these changes via fsnotify and reconciles automatically.
- **Config Bridge**: Implements a `config_bridge.go` that exposes an `AppConfig` DTO (Data Transfer Object) to the frontend. This DTO handles:
  - Field name mapping (e.g., camelCase for JSON/TypeScript, snake_case for TOML).
  - Command formatting: Converting `[]string` fields such as `args` and `stop_args` from TOML into single shell-like strings for user-friendly editing, and parsing them back using `util.ParseArgs`.
- **UI Conveniences**: Provides a native file dialog for choosing a command path, supports drag-and-drop registration of executable/script files, and derives accepted file extensions from the current Windows `PATHEXT` environment variable.
- **Termination**: The process terminates when the window is closed.

### Inter-Process Communication (IPC)

Communication between the core and UI processes follows two distinct paths:

1. **Status Updates (Core → UI)**: **Unidirectional** via standard I/O (stdio) using a synchronous, line-based JSON stream written by the core to the child UI process's `STDIN`. The core pushes process state updates (`MonitorStatus` JSON objects) to the UI.
2. **Configuration Changes (UI → Core via File)**: The UI does **not** send RPC commands back to the core to change state. Instead, it writes changes directly to `config.toml` using atomic operations (`util.SaveConfig`). The core picks up these changes via its `fsnotify` loop.

This design keeps the core simple (read config, reconcile) and preserves the "config.toml as the Single Source of Truth" principle.

## Reconciliation Loop

The core of the system is a **reconciliation loop** driven by file-system events. All config changes — whether from the user editing `config.toml` in a text editor or from the UI writing to it via `util.SaveConfig` — follow the same path: an atomic write on disk, an fsnotify event, a re-parse, and a reconcile pass.

```text
                  ┌──────────────────────┐
                  │   fsnotify watcher   │
                  │  (watches config.toml│
                  │   for Write events)  │
                  └──────────┬───────────┘
                             │ config changed
                             v
                  ┌──────────────────────┐
                  │   Load & Parse TOML  │
                  │  (validate config)   │
                  └──────────┬───────────┘
                             │ new desired state
                             v
                  ┌──────────────────────┐
                  │     Reconcile()      │
                  │                      │
                  │  Compare desired vs  │
                  │  current running     │
                  │  state and:          │
                  │                      │
                  │  • START new entries │
                  │  • STOP removed ones │
                  │  • RESTART modified  │
                  │  • NO-OP unchanged   │
                  └──────────────────────┘
```

### Reconciliation Algorithm

Given `desired` (from config.toml) and `current` (in-memory running state):

```text
for each entry in desired:
    if entry not in current:
        → START (new entry added to config)
    else if entry.enabled changed to false:
        → KILL process, STOP monitoring (disable)
    else if entry.enabled changed to true:
        → START monitoring (enable)
    else if process-identity fields changed (command, args, cwd, hide_window):
        → KILL existing process, then START with new config
    else if only monitoring-parameter fields changed:
        → HOT-RELOAD parameters in-place (no process restart)
    else:
        → NO-OP (already converged)

for each entry in current:
    if entry not in desired:
        → KILL process, STOP monitoring (entry removed from config)
```

> [!IMPORTANT]
> When an entry is removed from config.toml or disabled, the core **terminates (kills) the running process**. This ensures the actual system state always converges to what config.toml declares.

### Comparison Logic

To determine whether an entry's config has been "modified" (requiring restart), the reconciler compares all fields that affect runtime behavior. Note that parameters undergo **validation and default application** (`util.ValidateAndApplyDefaults`) before comparison:

- `command`, `args`, `cwd` — The process identity
- `hide_window` — Requires restart to change window visibility
- `restart_delay_sec`, `healthy_timeout_sec`, `max_retries`, `stop_command`, `stop_args`, `stop_timeout_sec` — Can be updated in-place (hot-reload) without restarting the monitored process

### File Editing & Debouncing

File-system events can fire multiple times for a single save operation (especially on Windows). The reconciler uses a debounce mechanism — after receiving an fsnotify event, it waits a short period (e.g. 500ms) for additional events before triggering reconciliation. This ensures that rapid successive writes (e.g. from a text editor) are coalesced into a single reconciliation pass.

> [!WARNING]
> Because `fsnotify` can trigger before a file is completely written, **Atomic Writes** are strongly required. When the UI or an external tool updates `config.toml`, it should write to a `.tmp` file first and perform an atomic rename/move. The core's debouncer should also handle occasional "File in use / Access Denied" errors gracefully by retrying after a short delay.

### Error Handling on Config Reload

If the new config file is **invalid** (parse error, malformed TOML), the core:

1. Logs the error.
2. **Keeps the current running state unchanged** — does not stop any processes.
3. Waits for the next valid config change.

This ensures that a typo in the config file does not accidentally kill running processes.

### Error Handling at Startup

Startup is intentionally stricter than live reload:

1. If `config.toml` does not exist, the core creates it with sample content and continues.
2. If the initial config load fails after that (parse error, validation failure, unreadable file), the core shows a native Windows error dialog and exits.
3. This prevents the tray from coming up in a partially initialized state with unknown monitoring behavior.

## Per-Process Monitor Lifecycle

Each monitored process entry runs through the following state machine:

```text
         ┌──────────┐
         │ Stopped  │ <────────────── (entry disabled / removed / core exit)
         └────┬─────┘
              │ enabled = true
              v
         ┌──────────┐
         │ Starting │
         └────┬─────┘
              │ spawn child (Job Object)
              v
         ┌──────────┐
         │ Running  │ <─┐ (healthy, retry count reset)
         └────┬─────┘   │
              │ process / tree exits
              v         │
         ┌──────────┐   │
         │ Retrying ├───┘ (wait & retry)
         └────┬─────┘
              │ max_retries exceeded
              v
         ┌──────────┐
         │ Failed   │
         └──────────┘
```

## Stop Command and Automatic Stop Detection

To stop a monitored process, Resurrector prefers an application-appropriate shutdown over an immediate `TerminateProcess`: if a `stop_command` is configured it is run first; otherwise Resurrector chooses a graceful mechanism automatically from observed runtime state. In both cases, `TerminateProcess` remains the final fallback after `stop_timeout_sec`.

### Config Fields

```toml
["My App"]
command = "myapp.exe"
enabled = true
stop_command = "taskkill"
stop_args = ["/PID", "${PID}", "/T"]
stop_timeout_sec = 5
```

- `stop_command` (String): Optional shutdown executable. Resolved via PATH if not an absolute path.
- `stop_args` (Array of Strings): Arguments passed to `stop_command`. Not shell-parsed.
- `stop_timeout_sec` (Integer): How long Resurrector waits for a graceful stop request to succeed before falling back to an explicit `TerminateProcess`. Default: `5`.
- `${PID}`: Placeholder expanded inside `stop_args` to the monitored root process PID at stop time. `${NAME}` expands an environment variable (rejected if undefined); `$$` produces a literal `$`. These placeholders are also supported in `command`, `args`, `cwd`, and `stop_command`, except that `${PID}` is only valid in `stop_args`.

### Semantics

- If `stop_command` is configured, Resurrector executes it first and then waits up to `stop_timeout_sec` for the monitored process to exit.
- If `stop_command` is omitted, Resurrector chooses the best-effort graceful stop method automatically from runtime observation.
- In every case, if the process is still alive after the timeout, Resurrector calls `TerminateProcess` to guarantee convergence.

### Stop Flow

```text
STOP requested
    |
    v
is stop_command configured?
    |
    +--> yes
    |      -> expand placeholders (${PID} and env vars)
    |      -> run stop_command
    |      -> wait stop_timeout_sec
    |      -> if exited: success
    |      -> else: call TerminateProcess
    |
    +--> no
           -> inspect runtime state
           -> if non-console top-level window exists: post WM_CLOSE
           -> else: try CTRL_BREAK_EVENT
           -> wait stop_timeout_sec
           -> if exited: success
           -> else: call TerminateProcess
```

### Design Notes

- The shutdown command is split into `stop_command` (executable string) and `stop_args` (argv array), matching the `command` / `args` pair, so config stays unambiguous and Windows quoting rules remain explicit.
- `stop_command` and `stop_args` are treated as argv, not shell syntax. If the user needs shell features, they should explicitly invoke `cmd.exe` or `powershell.exe`.
- The success condition of `stop_command` is not its exit code alone; the monitored process must actually exit.
- Automatic detection should be based on runtime observation rather than PE subsystem metadata alone. In particular, top-level window enumeration is more reliable than trying to classify the original command line statically.
- `WM_CLOSE` targeting should exclude both the classic `ConsoleWindowClass` and the ConPTY-era `PseudoConsoleWindow` class, so that a console host does not accidentally become the preferred stop target for a console-oriented process.
- `CTRL_BREAK_EVENT` remains narrower than Unix `SIGTERM`; it only works under specific console/process-group conditions, so it should be a best-effort fallback inside auto detection rather than a user-exposed mode.
- Even with graceful modes, an explicit `TerminateProcess` remains the final convergence mechanism. The design goal is **graceful first, forceful fallback**, not graceful-only shutdown.

### CTRL_BREAK_EVENT Delivery Details

To make `CTRL_BREAK_EVENT` actually reach the target child without side effects on the core process or other children, the implementation relies on three coordinated Windows behaviors:

1. **Child spawn flags**: Every monitored child is started with `CREATE_SUSPENDED | CREATE_NEW_PROCESS_GROUP`. Creating a new process group means the child's process-group ID is its own PID, and signals targeted at that group do not leak to the core process or to unrelated siblings.
2. **Targeted group, not group 0**: `GenerateConsoleCtrlEvent` is invoked with `dwProcessGroupId = child PID`, not `0`. This confines the event to the child and its descendants, so the core process does not need to install a `SetConsoleCtrlHandler` filter to ignore its own BREAK.
3. **Console attach handshake**: Before sending the event, the core calls `FreeConsole` (detach from any prior console) and then `AttachConsole(child PID)`. `GenerateConsoleCtrlEvent` requires the caller to share a console with the target. The attach is held for the full `stop_timeout_sec` window and released only after the child exits or the timeout elapses; detaching earlier can cause the event to be lost.

Because a Windows process can only be attached to **one** console at a time (the attach is per-process, not per-thread), concurrent `sendCtrlBreak` calls would race for the shared console state. The implementation serializes console attach/BREAK/detach across the package with a single mutex (`consoleAttachMu`). When many monitored children are stopped simultaneously, their `CTRL_BREAK_EVENT` deliveries run sequentially — each call waiting up to `stop_timeout_sec` — before escalating to `TerminateProcess`.

## Directory Structure

```text
.
├── build/                  # Build artifacts (generated binaries, etc.)
├── core/                   # Resident core process (Pure Go)
│   ├── main.go             # Entry point, fsnotify watcher setup
│   ├── reconciler.go       # Reconciliation logic (desired vs actual state)
│   ├── monitor.go          # Per-process monitoring (start, wait, graceful stop via WM_CLOSE / CTRL_BREAK_EVENT / TerminateProcess)
│   ├── autostart.go        # Windows logon auto-start toggle (HKCU Run key)
│   ├── tray.go             # System tray control
│   └── ipc.go              # UI process communication control
├── ui/                     # UI process (Wails)
│   ├── main.go             # Wails entry point
│   ├── app.go              # Wails lifecycle and IPC setup
│   ├── config_bridge.go    # Bridge for CRUD operations on config.toml
│   └── frontend/           # Svelte application (UI screens)
├── util/                   # Common utilities
│   ├── config.go           # TOML parsing, Atomic writes, Shell-like arg parsing
│   ├── expand.go           # ${NAME} / ${PID} / $$ placeholder expansion
│   ├── flag.go             # Shared CLI flag parsing and config-path normalization
│   └── logger.go           # Shared slog setup (output destination and format)
├── doc/                    # Design docs and rationales
├── misc/                   # Standalone test programs for verifying stop behavior (e.g. wait_for_os_interrupt, alert_on_wm_close)
├── config.example.toml     # Sample configuration file
└── package.json            # Build scripts (npm)
```

## Development

### Prerequisites

- Go 1.26+
- Node.js 22+ (LTS recommended)
- Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
