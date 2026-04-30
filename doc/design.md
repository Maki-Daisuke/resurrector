# Resurrector Design & Architecture

This document covers the internal architecture and implementation details of Resurrector — the _what_ and _how_. For the reasoning behind key design decisions, see [Design Rationales](./rationales.md).

## System Overview

```text
+------------------------------------------------------------------+
|  Interactive Session (Windows Desktop)                           |
|                                                                  |
|            ~/.config/resurrector/config.toml                     |
|                  (Single Source of Truth)                         |
|                    |                ^                             |
|         (fsnotify) |                | (direct write)             |
|           (read)   v                |                            |
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

`config.toml` is the **Single Source of Truth**: both the user and the UI write to it, and the core reads from it. The core never writes to it after first-run bootstrap. All config changes — from a text editor or the UI — flow through the same fsnotify-driven reconciliation path.

## Components

Resurrector consists of two independent binaries: a **resident core process** and a **disposable UI process**.

### Core Process (`resurrector.exe`)

- **Role**: Steady presence in the system tray. Watches `config.toml` for changes (read-only) and reconciles monitored processes to match the desired state. By default it reads from `~/.config/resurrector/config.toml`, but accepts CLI flags such as `-f <path>`, `-log-file <path>`, and `-log-format <text|json>`.
- **Technology**: Go (Pure), `energye/systray`, `golang.org/x/sys/windows`, `github.com/fsnotify/fsnotify`
- **Startup**: On first launch, if `config.toml` does not exist, the core bootstraps the file with sample content. A session-local named mutex prevents more than one core instance per Windows session; a second launch shows an error dialog and exits.
- **Tray**: Launches the UI process as a child process when "Open Settings" is clicked, passing runtime flags (`-f`, `-log-file`, `-log-format`) for consistent behavior. Can toggle logon auto-start for the current user via `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.

### UI Process (`resurrector-ui.exe`)

- **Role**: Configuration screen and real-time monitoring status display. Provides CRUD access to `config.toml` entries via a Wails bridge.
- **Technology**: Go + Wails v2 + Svelte 5 (runes mode, TypeScript) + Tailwind CSS v4 + AG Grid (Community)
- **Config Bridge** (`config_bridge.go`): Exposes an `AppConfig` DTO to the frontend, handling field-name mapping (camelCase ↔ snake_case) and converting `[]string` fields such as `args` and `stop_args` into shell-like strings for editing, with `util.ParseArgs` for the reverse parse.
- **Conveniences**: Native file dialog for choosing a command path, drag-and-drop registration of executables, file-extension filtering derived from `PATHEXT`.
- **Writes**: The UI writes directly to `config.toml` using atomic operations (`util.SaveConfig`). The core detects the change via fsnotify and reconciles automatically. The UI never sends RPC commands to the core.
- **Termination**: The process exits when the window is closed.

### Inter-Process Communication (IPC)

Communication follows two distinct, intentionally narrow paths:

1. **Status updates (Core → UI)**: Unidirectional, via the UI child process's `STDIN`. The core writes a synchronous, line-based JSON stream of `MonitorStatus` objects.
2. **Config changes (UI → Core)**: The UI does **not** send commands to the core. It writes `config.toml` atomically; the core picks up the change via fsnotify.

This keeps the core simple (read config, reconcile) and avoids a second state-mutation interface alongside the config file.

## Reconciliation

The core drives a **reconciliation loop** on every config file change. All config writes — from a text editor or the UI — follow the same path: atomic write on disk → fsnotify event → re-parse → reconcile pass.

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

### Algorithm

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

Fields are compared after **validation and default application** (`util.ValidateAndApplyDefaults`):

- `command`, `args`, `cwd`, `hide_window` — **Process-identity fields**: any change requires killing and restarting the process.
- `restart_delay_sec`, `healthy_timeout_sec`, `max_retries`, `stop_command`, `stop_args`, `stop_timeout_sec` — **Monitoring-parameter fields**: updated in-place (hot-reload) without restarting the monitored process.

### Debouncing

File-system events can fire multiple times for a single save (especially on Windows). The reconciler debounces events — after receiving an fsnotify event, it waits a short period (e.g. 500 ms) for additional events before triggering reconciliation, coalescing rapid successive writes into a single pass.

> [!WARNING]
> Because `fsnotify` can trigger before a file is completely written, **atomic writes are required**. Write to a `.tmp` file first, then perform an atomic rename/move. The core's debouncer handles occasional "File in use / Access Denied" errors by retrying after a short delay.

### Error Handling

**At startup** (strict):

1. If `config.toml` does not exist, the core creates it with sample content and continues.
2. If the initial load fails (parse error, validation failure, unreadable file), the core shows a native Windows error dialog and exits.

**During live reload** (lenient):

1. If the new config is invalid, the core logs the error and keeps the current running state unchanged.
2. No processes are stopped. The core waits for the next valid config change.

## Config File Format

`util.SaveConfig` writes `config.toml` to stay **human-friendly** and **diff-friendly** across UI-driven writes:

- **Default values are omitted.** Fields equal to their documented defaults (e.g. `enabled = true`, `max_retries = -1`, `stop_timeout_sec = 5`) are not written. `command` is always present (mandatory, no default). `max_retries = 0` is a meaningful value ("no retry") distinct from the default, and is written when set explicitly.
- **Fields are written in a fixed semantic order**: `command`, `args`, `cwd`, `enabled`, `hide_window`, `stop_command`, `stop_args`, `stop_timeout_sec`, `restart_delay_sec`, `max_retries`, `healthy_timeout_sec`. This is enforced via a dedicated `appTOML` struct (preserving declaration order in `go-toml/v2`). Table headers are sorted alphabetically by app name.

Together, these rules give **round-trip stability**: saving a just-loaded config produces byte-identical output (modulo user comments and whitespace), minimizing spurious fsnotify events and keeping version-controlled configs reviewable.

## Process Monitoring

### Lifecycle

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

### Stop Behavior

Resurrector prefers graceful shutdown over immediate `TerminateProcess`. `TerminateProcess` remains the final fallback after `stop_timeout_sec` in all cases.

**Stop flow:**

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

**Notes:**

- `stop_command` + `stop_args` mirror `command` + `args`: argv-based, not shell-parsed. Use `cmd.exe` / `powershell.exe` explicitly if shell features are needed.
- Success is measured by whether the monitored process actually exits, not by the exit code of `stop_command`.
- Auto-detection is based on runtime window enumeration, not PE subsystem metadata. `WM_CLOSE` targets exclude `ConsoleWindowClass` and `PseudoConsoleWindow` to avoid accidentally targeting a console host.
- `CTRL_BREAK_EVENT` is a best-effort fallback, not a user-exposed mode. See [CTRL_BREAK_EVENT Delivery](#ctrl_break_event-delivery) for implementation constraints.
- The goal is **graceful first, forceful fallback** — not graceful-only shutdown.

## Windows Implementation Notes

### Job Objects

Every monitored child is started with `CREATE_SUSPENDED | CREATE_NEW_PROCESS_GROUP` and immediately assigned to a Windows Job Object (`CreateJobObject`, `AssignProcessToJobObject`). This binds the child's entire process tree to the core's lifetime: if Resurrector exits, all monitored processes are guaranteed to terminate with it — including sub-processes spawned by commands like `npm run start`.

### CTRL_BREAK_EVENT Delivery

Delivering `CTRL_BREAK_EVENT` to a specific child without side effects on the core or other children requires three coordinated Windows behaviors:

1. **New process group at spawn**: Each child is started with `CREATE_NEW_PROCESS_GROUP`, so its process-group ID equals its own PID. Signals targeted at that group do not reach the core or unrelated siblings.
2. **Targeted `GenerateConsoleCtrlEvent`**: Called with `dwProcessGroupId = child PID` (not `0`), confining the event to the child and its descendants.
3. **Console attach handshake**: Before sending the event, the core calls `FreeConsole` then `AttachConsole(child PID)` — `GenerateConsoleCtrlEvent` requires the caller to share a console with the target. The attach is held for the full `stop_timeout_sec` window and released only after the child exits or the timeout elapses.

Because a process can only be attached to **one** console at a time, concurrent `sendCtrlBreak` calls are serialized with a package-level mutex (`consoleAttachMu`). When many monitored children are stopped simultaneously, their `CTRL_BREAK_EVENT` deliveries run sequentially before escalating to `TerminateProcess`.

## Supported OS

| Item             | Requirement                                                                                                                                                                                                                       |
| ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| OS               | **Windows 10 version 1809 (October 2018 Update) or later**, or **Windows 11**. Windows Server 2019 / 2022 / 2025 also work.                                                                                                       |
| Architecture     | **x64 (amd64)** only. 32-bit (x86) and ARM64 builds are not provided.                                                                                                                                                             |
| WebView2 Runtime | Required for the management UI. Pre-installed on Windows 11; auto-distributed to most Windows 10 devices via Windows Update. Otherwise install the [Evergreen Runtime](https://developer.microsoft.com/microsoft-edge/webview2/). |

### Why Windows 10 1809+?

The minimum version is determined by the most recent API requirement across the stack:

- **Nested Job Objects.** Resurrector binds each child to a Job Object with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`. Job Objects themselves exist since Windows 2000, but **nested Job Objects** — needed when the monitored process already runs inside a Job Object (browsers, IDEs, Task Scheduler tasks) — require **Windows 8 or later**.
- **ConPTY-aware graceful shutdown.** The graceful-stop path skips sending `WM_CLOSE` to windows with the `PseudoConsoleWindow` class, which was introduced by [ConPTY](https://devblogs.microsoft.com/commandline/windows-command-line-introducing-the-windows-pseudo-console-conpty/) in **Windows 10 version 1809**. Without this guard, closing a ConPTY pseudo-console window would terminate the console host rather than the intended app.
- **WebView2 / Wails v2.** The management UI renders via Microsoft Edge WebView2. Microsoft [ended WebView2 support for Windows 7 SP1 and Windows 8.1 on October 10, 2023](https://learn.microsoft.com/lifecycle/announcements/webview2-end-of-support-windows-7-8-81), making Windows 10/11 the only supported baseline.

Windows 7, 8, 8.1, and Windows 10 builds older than 1809 are **not supported** and may fail to start or exhibit degraded behavior.

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
│   └── frontend/           # Svelte 5 (runes mode) + TypeScript + Tailwind CSS v4
│       └── src/
│           ├── App.svelte   # Main window: process status table and edit dialog
│           ├── AgGrid.svelte# Thin wrapper around AG Grid's vanilla createGrid API
│           ├── main.ts      # Mounts App via svelte#mount and registers AG Grid modules
│           └── style.css    # Tailwind entrypoint with inline @theme / @custom-variant config
├── util/                   # Common utilities
│   ├── config.go           # TOML parsing, Atomic writes, Shell-like arg parsing
│   ├── expand.go           # ${NAME} / ${PID} / $$ placeholder expansion
│   ├── flag.go             # Shared CLI flag parsing and config-path normalization
│   └── logger.go           # Shared slog setup (output destination and format)
├── doc/                    # Design docs and rationales
├── misc/                   # Standalone test programs for verifying stop behavior (e.g. wait_for_os_interrupt, alert_on_wm_close)
└── package.json            # Build scripts (pnpm)
```

For build prerequisites and instructions, see [Building from source](../README.md#building-from-source) in the README.
