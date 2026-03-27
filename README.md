# Resurrector

Resurrector is a lightweight process monitoring and auto-restart tool for Windows.

It is designed to ensure that critical applications remain running within the **Interactive Session** (the desktop session where you are logged in), effortlessly resurrecting (restarting) them if they crash or terminate unexpectedly.

## Features

- **Strict Lifecycle Control**: Resurrector completely owns the lifecycle of monitored apps. It spawns them as child processes bound to a **Windows Job Object**, guaranteeing that apps (and their subprocesses) cleanly terminate when stopped.
- **Config-as-Code (SSoT)**: `config.toml` acts as the definitive Single Source of Truth. The core uses `fsnotify` to track changes in real-time and reconciles the system state automatically. Changes via the UI or external editors are applied instantly without restarting the core.
- **Zero-Polling Monitoring**: Event-driven monitoring using Windows API (`WaitForSingleObject`). It does not waste CPU resources.
- **Minimal Footprint**: The resident core process is written in pure Go and consumes only a few megabytes of memory.
- **On-Demand Modern UI**: The configuration and status UI (Wails + Svelte) only launches when called from the system tray. It exits and frees all memory when not needed.

For more technical details about the architecture, IPC, and reconciliation loop, please see [Design & Architecture](./doc/design.md).

## Configuration (`config.toml`)

Applications to be monitored are managed via a `config.toml` file located in the same directory as the executable.

> [!WARNING]
> Because Resurrector watches this file for real-time changes using `fsnotify`, **Atomic Writes** are required if modified by external tools. Edit directly using a standard text editor, or if scripting writes, write to a `.tmp` file and perform an atomic rename/move.

```toml
# Resurrector Configuration

["PowerToys Awake"]
enabled = true
command = 'C:\Program Files\PowerToys\modules\Awake\PowerToys.Awake.exe'
args = ["--use-pt-config"]
cwd = 'C:\Program Files\PowerToys\modules\Awake'
restart_delay_sec = 3
healthy_timeout_sec = 60
hide_window = true
max_retries = 5

["My Svelte Dev Server"]
enabled = false
command = 'npm.cmd'
args = ["run", "dev"]
cwd = 'C:\Users\user\projects\my-svelte-app'
restart_delay_sec = 5
healthy_timeout_sec = 60
hide_window = false
max_retries = 3
```

### Item Definitions

- `name` (String): The identifier name displayed on the UI.
- `enabled` (Boolean): If `true`, starts monitoring on startup or UI request.
- `command` (String): The full path to the command or executable.
- `args` (Array of Strings): List of arguments to pass to the command.
- `cwd` (String): The working directory (current directory) for running the command.
- `restart_delay_sec` (Integer): The wait time (seconds) before attempting a restart after detecting a process termination.
- `healthy_timeout_sec` (Integer): If the process continues to run stably for this many seconds after a restart, the retry count is reset to 0.
- `hide_window` (Boolean): If `true`, launches the process in the background (hidden window).
- `max_retries` (Integer): The maximum number of restarts before stopping monitoring due to persistent crashes (crash loop prevention).

## Build

To build the entire project, run the following commands in the root directory:

```bash
npm install
npm run build
```

The following files will be generated in the `build/` directory:

- `resurrector.exe` (Core Process)
- `resurrector-ui.exe` (UI Process)
- `config.toml` (Configuration file - copied from `config.example.toml`)

## Usage

1. Run `build/resurrector.exe`.
2. An icon will appear in the system tray.
3. Right-click the icon and select "Settings" to launch the UI for configuring monitored applications.

## Development

### Prerequisites

- Go 1.26+
- Node.js 22+ (LTS recommended)
- Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Author

Daisuke (yet another) Maki
