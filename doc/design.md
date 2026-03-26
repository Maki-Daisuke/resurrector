# Resurrector Design & Architecture

This document provides detailed information about the internal design, architecture, and development environment of Resurrector.

## System Overview

```text
+-------------------------------------------------------+
|  Interactive Session (Windows Desktop)                |
|                                                       |
|  +-------------------+         +-------------------+  |
|  |   Core Process    |  (IPC)  |    UI Process     |  |
|  | (resurrector.exe) |<------->| (resurrector-ui)  |  |
|  |                   |  stdio  | [Wails + Svelte]  |  |
|  +---------+---------+         +-------------------+  |
|            |                                          |
|            | (Monitor / Restart)                      |
|            v                                          |
|  +-------------------+                                |
|  | Monitored Process |                                |
|  | (e.g. PowerToys)  |                                |
|  +-------------------+                                |
|                                                       |
+-------------------------------------------------------+
```

## Architecture

To minimize system resource consumption, Resurrector consists of two independent binaries: a **"resident core process"** and a **"disposable UI process."**

### 1. Core Process (`resurrector.exe`)

- **Role**: Steady presence in the system tray, reading the TOML file, and starting/monitoring child processes.
- **Technology**: Go (Pure), `energye/systray`, `golang.org/x/sys/windows`, `github.com/shirou/gopsutil`
- **Features**: Does not have a UI; it continues to operate extremely lightly. When "Settings" is clicked from the system tray, it launches the UI process as a child process. Process detection and matching is robustly handled using `gopsutil`.

### 2. UI Process (`resurrector-ui.exe`)

- **Role**: Configuration screen for the user, real-time display of monitoring status.
- **Technology**: Go + Wails + Svelte (TypeScript)
- **Features**: Uses a custom Wails logger that writes to `STDERR`, leaving `STDOUT` and `STDIN` exclusively for JSON-RPC messaging (IPC). The process terminates when the window is closed.

### Robust Inter-Process Communication (IPC)

Communication between the core and UI processes is handled via standard I/O (stdio) using Go's standard `net/rpc/jsonrpc` package. This makes it secure by not opening any network ports. The core process spawns the UI process and communicates through pipes, ensuring a low-latency and secure link.

## Directory Structure

```text
.
├── build/                  # Build artifacts (generated binaries, etc.)
├── core/                   # Resident core process (Pure Go)
│   ├── main.go             # Entry point
│   ├── monitor.go          # Process monitoring logic
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
