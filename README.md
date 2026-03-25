# Resurrector

Resurrector is a lightweight process monitoring and auto-restart tool for Windows.

It is designed to ensure that critical applications remain running within the **Interactive Session** (the desktop session where you are logged in), effortlessly resurrecting (restarting) them if they crash or terminate unexpectedly.

## Features

- **Zero-Polling Monitoring**: Event-driven monitoring using Windows API (`WaitForSingleObject`). It does not waste CPU resources.
- **Minimal Footprint**: The resident core process is written in pure Go and consumes only a few megabytes of memory.
- **On-Demand Modern UI**: The configuration and status UI (Wails + Svelte) only launches when called from the system tray. It exits and frees all memory when not needed.
- **Robust Inter-Process Communication (IPC)**: Communication between the core and UI processes is handled via standard I/O (stdio), making it secure by not opening any network ports.
- **Human-Readable Configuration**: Uses the `TOML` format for easy reading and writing.

## Architecture

To minimize system resource consumption, Resurrector consists of two independent binaries: a **"resident core process"** and a **"disposable UI process."**

### 1. Core Process (`resurrector.exe`)

- **Role**: Steady presence in the system tray, reading the TOML file, and starting/monitoring child processes.
- **Technology**: Go (Pure), `energye/systray`, `golang.org/x/sys/windows`
- **Features**: Does not have a UI; it continues to operate extremely lightly. When "Settings" is clicked from the system tray, it launches the UI process as a child process.

### 2. UI Process (`resurrector-ui.exe`)

- **Role**: Configuration screen for the user, real-time display of monitoring status.
- **Technology**: Go + Wails + Svelte (TypeScript)
- **Features**: Uses a custom Wails logger to utilize `STDOUT` as a dedicated pipe for pure JSON messaging (IPC). The process terminates when the window is closed.

## Configuration (`config.toml`)

Applications to be monitored are managed via a `config.toml` file located in the same directory as the executable.

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

### Directory Structure

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

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Author

Daisuke (yet another) Maki
