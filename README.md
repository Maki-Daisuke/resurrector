# Resurrector

> **Never let your critical Windows applications stay dead.**

Resurrector is a lightweight process monitoring and auto-restart tool for Windows.

It is designed to ensure that critical applications remain running within the **Interactive Session** (the desktop session where you are logged in), effortlessly resurrecting (restarting) them if they crash or terminate unexpectedly.

## Features

- **Strict Lifecycle Control**: Resurrector completely owns the lifecycle of monitored apps. It spawns them as child processes bound to a **Windows Job Object**, guaranteeing that apps (and their subprocesses) cleanly terminate when stopped.
- **Config-as-Code (SSoT)**: `config.toml` acts as the definitive Single Source of Truth. The core uses `fsnotify` to track changes in real-time and reconciles the system state automatically. Changes via the UI or external editors are applied instantly without restarting the core.
- **Modern Management UI**: The configuration provides a user-friendly way to manage monitored applications. The UI only launches when called from the system tray and frees all memory when closed.
- **Windows-Native Integration**: The tray menu can toggle auto-start at Windows sign-in for the current user, and the core enforces a single running instance per Windows session.
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
- `enabled` (Boolean): If `true`, starts monitoring on startup or UI request. If omitted, it defaults to `false`.
- `hide_window` (Boolean): If `true`, launches the process in the background (hidden window). (Default: `false`)
- `restart_delay_sec` (Integer): The wait time (seconds) before attempting a restart after detecting a process termination. (Default: `0`)
- `max_retries` (Integer): Retry count limit for a crashing app. `0` means no retry; negative values mean infinite retry. (Default: `-1` / Infinite retry)
- `healthy_timeout_sec` (Integer): If the process runs for at least this many seconds before exiting, the restart counter is reset to 0. `0` disables that reset (the counter increments on every exit). (Default: `0`)
- `stop_command` (String): Optional shutdown executable. If a relative path or just a command name (e.g., `taskkill`) is provided, Resurrector will attempt to resolve the absolute path using the system PATH. (Default: `""`)
- `stop_args` (Array of Strings): List of arguments passed to `stop_command`. Not shell-parsed. If shell features are needed, explicitly invoke a shell such as `cmd.exe` or `powershell.exe` as the `stop_command`. The `{pid}` placeholder in each argument is replaced with the monitored root process PID before execution. (Default: `[]`)
- `stop_timeout_sec` (Integer): How long Resurrector waits for graceful shutdown attempts before falling back to `TerminateProcess`. (Default: `5`)

### Stop Behavior

If `stop_command` is specified, Resurrector runs it first and waits up to `stop_timeout_sec` for the monitored process to exit.

```toml
["My App"]
command = "myapp.exe"
enabled = true
stop_command = "taskkill"
stop_args = ["/PID", "{pid}", "/T"]
stop_timeout_sec = 5
```

If `stop_command` is not specified, Resurrector chooses the best-effort graceful stop method automatically from runtime observation:

1. If the target PID owns a non-console top-level window (excluding both the classic console host and ConPTY pseudo-console windows), post `WM_CLOSE`.
2. Otherwise, send `CTRL_BREAK_EVENT` to the child's process group.
3. If the process is still alive after `stop_timeout_sec`, call `TerminateProcess`.

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
3. Right-click the icon and select "Open Settings" to launch the UI for configuring monitored applications.

### Tray Menu

- `Open Settings`: Launches the UI process. If the UI is already open, the core does not launch a second copy.
- `Auto-start Resurrector`: Toggles Windows sign-in auto-start for the current user by updating `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.
- `Quit`: Stops all monitored processes owned by Resurrector and exits the core.

### Startup Behavior

- Only one core instance can run per Windows session. If you launch a second copy, Resurrector shows an error dialog and exits.
- On first launch, if the config file does not exist, Resurrector creates it with sample content automatically.
- If the config file is invalid during startup, Resurrector shows an error dialog and exits.
- If the config file becomes invalid later while the core is already running, the current monitored state is kept and the change is ignored until the file is fixed.

### UI Conveniences

- The UI lets you edit `args` and `stop_args` as single shell-like strings instead of TOML string arrays.
- You can browse for a `command` or `stop_command` path with a native file dialog.
- You can drag and drop an executable or script file onto the UI window to start creating a new app entry.
- Accepted command/script file extensions follow the current Windows `PATHEXT` environment variable.

### CLI Options

- `-f <path>`: Specifies a custom path to the `config.toml` file. If not provided, it defaults to `%USERPROFILE%\.config\resurrector\config.toml`.
- `-log-file <path>`: Writes logs to the specified file in append mode. If not provided, logs are written to `stderr`.
- `-log-format <text|json>`: Sets the log output format. Default is `text`.

## Development

### Prerequisites

- Go 1.26+
- Node.js 22+ (LTS recommended)
- Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Author

Daisuke (yet another) Maki
