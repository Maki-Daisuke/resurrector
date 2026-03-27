# Resurrector Design & Architecture

This document provides detailed information about the internal design, architecture, and development environment of Resurrector.

## Design Philosophy

> **config.toml is the Single Source of Truth (SSoT).**

Resurrector treats `config.toml` as a **declarative desired-state specification** — similar to how Kubernetes treats manifests, or how Terraform treats `.tf` files. The running system continuously observes the config file and **reconciles** the actual process state to match the desired state described in it.

### Key Principles

1. **Declarative over Imperative**: Users declare *what* should be running, not *how* to transition. The core reconciles the difference automatically.
2. **Idempotent Reconciliation**: Applying the same config file repeatedly produces the same result. The reconciler compares desired state vs. actual state and only performs necessary actions.
3. **File-Watch Driven**: The core uses `fsnotify` to detect config file changes in real-time. No restart of the core process is required to pick up configuration changes.
4. **Graceful Convergence**: When config changes, the core handles additions, removals, and modifications of entries gracefully — starting new processes, stopping removed ones, and restarting modified ones as needed.
5. **Core is Read-Only**: The core process **never writes** to `config.toml`. It only reads and reacts. All config modifications are performed externally — either by the user editing the file directly or by the UI process writing to it.
6. **Strict Lifecycle Ownership**: Resurrector entirely owns the lifecycle of the monitored processes. It launches them as child processes bound to a **Windows Job Object**. If Resurrector exits, all monitored processes are guaranteed to terminate with it.
7. **Atomic Config Updates**: Modifications to `config.toml` (especially by the UI) must be performed atomically (e.g., write to a temporary file, then rename/move) to prevent the core from reading partially written files.

## System Overview

```text
+------------------------------------------------------------------+
|  Interactive Session (Windows Desktop)                           |
|                                                                  |
|                       config.toml                                |
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

- **Role**: Steady presence in the system tray. Watches `config.toml` for changes (read-only) and reconciles monitored processes to match the desired state.
- **Technology**: Go (Pure), `energye/systray`, `golang.org/x/sys/windows`, `github.com/shirou/gopsutil`, `github.com/fsnotify/fsnotify`
- **Features**: Does not have a UI; it continues to operate extremely lightly. When "Settings" is clicked from the system tray, it launches the UI process as a child process. **Never writes to `config.toml`.**
- **Process Management**: Uses Windows Job Objects (`CreateJobObject`, `AssignProcessToJobObject`) to bind spawned child processes to its own lifecycle. This ensures that any command (even ones that spawn further sub-processes like `npm run start`) and its entire process tree are cleanly terminated when the core requests a stop or when the core process itself exits.

### 2. UI Process (`resurrector-ui.exe`)

- **Role**: Configuration screen for the user, real-time display of monitoring status. Edits `config.toml` directly.
- **Technology**: Go + Wails + Svelte (TypeScript)
- **Features**: Uses a custom Wails logger that writes to `STDERR`, leaving `STDOUT` and `STDIN` exclusively for JSON-RPC messaging (IPC). The UI **writes directly to `config.toml`** when the user makes configuration changes — the core detects these changes via fsnotify and reconciles automatically. The process terminates when the window is closed.

### Inter-Process Communication (IPC)

Communication between the core and UI processes is **unidirectional** (core → UI) via standard I/O (stdio) using Go's standard `net/rpc/jsonrpc` package. The core pushes process state updates to the UI. The UI does **not** send commands back to the core — instead, it writes configuration changes directly to `config.toml`, and the core picks them up via fsnotify.

This design keeps the core simple (read config, reconcile) and avoids bidirectional RPC complexity.

## Reconciliation Loop

The core of the system is a **reconciliation loop** driven by file-system events:

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

To determine whether an entry's config has been "modified" (requiring restart), the reconciler compares all fields that affect runtime behavior:

- `command`, `args`, `cwd` — The process identity
- `hide_window` — Requires restart to change window visibility
- `restart_delay_sec`, `healthy_timeout_sec`, `max_retries` — Can be updated in-place (hot-reload) without restarting the monitored process

> [!NOTE]
> Fields like `restart_delay_sec`, `healthy_timeout_sec`, and `max_retries` are **monitoring parameters**, not process identity fields. Changing them does NOT require stopping and restarting the monitored process. They can be hot-reloaded by updating the in-memory config reference.

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

## Data Flow

```text
                    config.toml (SSoT)
                    ^              ^
        (fsnotify)  |              |  (direct write)
          (read)    |              |
    +---------------+---+     +----+--------------+
    |  Core (Reconciler)|     |   UI Process      |
    +--------+----------+     +-------------------+
             |          state push (IPC)
             |---------------------------->
             |
             | (Reconcile: Start/Kill/Restart)
             v
    +-------------------+
    | Monitored Process |
    +-------------------+
```

### Config Modification Flow

All config changes — whether from the user editing the file in a text editor or through the UI — follow the **same code path**:

1. `config.toml` is modified using an **atomic write** (e.g., `rename(config.toml.tmp, config.toml)`).
2. The core's `fsnotify` watcher detects the file change.
3. The core re-reads and parses the config.
4. The reconciler compares desired state vs. actual state and converges.

> [!NOTE]
> The core process **never writes** to `config.toml`. The UI process handles config editing by writing to the file directly. This keeps the core's responsibility clean and simple: **read → reconcile → monitor**.

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

## Directory Structure

```text
.
├── build/                  # Build artifacts (generated binaries, etc.)
├── core/                   # Resident core process (Pure Go)
│   ├── main.go             # Entry point, fsnotify watcher setup
│   ├── reconciler.go       # Reconciliation logic (desired vs actual state)
│   ├── monitor.go          # Per-process monitoring (start, attach, wait)
│   ├── tray.go             # System tray control
│   └── ipc.go              # UI process communication control
├── ui/                     # UI process (Wails)
│   ├── main.go             # Wails entry point
│   ├── app.go              # Bridge between Wails and frontend
│   └── frontend/           # Svelte application (UI screens)
├── util/                   # Common utilities
│   ├── config.go           # TOML configuration reading/writing
│   └── stdioconn.go        # IPC via standard I/O
├── config.example.toml     # Sample configuration file
└── package.json            # Build scripts (npm)
```

## Development

### Prerequisites

- Go 1.26+
- Node.js 22+ (LTS recommended)
- Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
