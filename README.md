# Resurrector

> **Never let your critical Windows applications stay dead.**

Resurrector is a lightweight process monitoring and auto-restart tool for Windows.

It is designed to ensure that critical applications remain running within the **Interactive Session** (the desktop session where you are logged in), effortlessly resurrecting (restarting) them if they crash or terminate unexpectedly.

## Features

- **Strict Lifecycle Control**: Resurrector completely owns the lifecycle of monitored apps. It spawns them as child processes bound to a **Windows Job Object**, guaranteeing that apps (and their subprocesses) cleanly terminate when stopped.
- **Config-as-Code (SSoT)**: `config.toml` acts as the definitive Single Source of Truth. The core uses `fsnotify` to track changes in real-time and reconciles the system state automatically. Changes via the UI or external editors are applied instantly without restarting the core.
- **Modern Management UI**: The configuration provides a user-friendly way to manage monitored applications. The UI only launches when called from the system tray and frees all memory when closed.
- **Zero-Polling Monitoring**: Event-driven monitoring using Windows API (`WaitForSingleObject`). It does not waste CPU resources.
- **Minimal Footprint**: The resident core process is written in pure Go and consumes only a few megabytes of memory.

For more technical details about the architecture, IPC, and reconciliation loop, please see [Design & Architecture](./doc/design.md).

## Configuration (`config.toml`)

Applications to be monitored are managed via a `config.toml` file. By default, this is located in `%USERPROFILE%\.config\resurrector\config.toml`. The file is automatically created with sample content when you launch the core application for the first time. You can also specify a custom configuration file path using the `-f <path>` command-line argument.

> [!WARNING]
> Because Resurrector watches this file for real-time changes using `fsnotify`, **Atomic Writes** are required if modified by external tools. The built-in UI handles this automatically. If using external scripts, write to a `.tmp` file and perform an atomic rename/move.
>
> [!TIP]
> While the TOML file requires arguments as a string array (`args = ["-a", "-b"]`), the **Management UI** allows you to enter them as a single shell-like string (e.g. `-a -b "quoted string"`), which it then parses automatically.

```toml
# Resurrector Configuration

["PowerToys Awake"]
command = 'C:\Program Files\PowerToys\modules\Awake\PowerToys.Awake.exe'
args = ["--use-pt-config"]
cwd = 'C:\Program Files\PowerToys\modules\Awake'
enabled = true
hide_window = true
restart_delay_sec = 3
max_retries = 5
healthy_timeout_sec = 60

["My Svelte Dev Server"]
command = 'npm'
args = ["run", "dev"]
cwd = 'C:\Users\user\projects\my-svelte-app'
enabled = false
```

### Item Definitions

- `[name]` (String): The identifier name displayed on the UI.
- `command` (String): The full path to the command or executable. **(Mandatory)**. If a relative path or just a command name (e.g., `npm`) is provided, Resurrector will attempt to resolve the absolute path using the system PATH.
- `args` (Array of Strings): List of arguments to pass to the command. (Default: `[]`)
- `cwd` (String): The working directory (current directory) for running the command. (Default: The directory where the resolved `command` is located)
- `enabled` (Boolean): If `true`, starts monitoring on startup or UI request. **(Mandatory)**
- `hide_window` (Boolean): If `true`, launches the process in the background (hidden window). (Default: `false`)
- `restart_delay_sec` (Integer): The wait time (seconds) before attempting a restart after detecting a process termination. (Default: `0`)
- `max_retries` (Integer): The maximum number of restarts before stopping monitoring due to persistent crashes (crash loop prevention). (Default: `0` / Infinite retry)
- `healthy_timeout_sec` (Integer): If the process continues to run stably for this many seconds after a restart, the retry count is reset to 0. (Default: `0` / Infinite retry)

## Build

To build the entire project, run the following commands in the root directory:

```bash
npm install
npm run build
```

The following files will be generated in the `build/` directory:

- `resurrector.exe` (Core Process)
- `resurrector-ui.exe` (UI Process)

## Usage

1. Run `build/resurrector.exe`.
2. An icon will appear in the system tray.
3. Right-click the icon and select "Settings" to launch the UI for configuring monitored applications.

### CLI Options

- `-f <path>`: Specifies a custom path to the `config.toml` file. If not provided, it defaults to `%USERPROFILE%\.config\resurrector\config.toml`.

## Development

### Prerequisites

- Go 1.26+
- Node.js 22+ (LTS recommended)
- Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Author

Daisuke (yet another) Maki
