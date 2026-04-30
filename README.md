# Resurrector

> **Resurrect what Windows won't.**
> _Per-process supervision for the apps Windows services can't reach._

Resurrector is a lightweight tool for Windows that **launches, monitors, and auto-restarts** the applications you tell it to keep alive.

It is designed to ensure that critical applications remain running within the **Interactive Session** (the desktop session where you are logged in), effortlessly resurrecting (restarting) them if they crash or terminate unexpectedly.

![Management UI](https://github.com/user-attachments/assets/adf4f2b3-1ec4-4c87-b8a2-191053bf6aa1)

## Why Resurrector?

Crashy background apps, dev servers that die silently, utilities that need to stay alive — Resurrector keeps them all up without getting in your way.

On Linux, `systemd` provides robust process supervision out of the box. On Windows, the equivalent — the **Service Control Manager** — only covers Windows services, which run in an isolated session and cannot interact with the desktop. For everyday apps that need to live in your **Interactive Session** (your logged-in desktop) — dev servers, tray utilities, GUI tools — there are surprisingly few supervision options, and the ones that exist (e.g. ["Restart on Crash"](https://w-shadow.com/blog/2009/03/04/restart-on-crash/)) only watch by **executable name**. That breaks down the moment two apps share an executable: two `node.exe` processes for two different projects look identical to a name-based watcher.

Resurrector fills that gap with **true process-level supervision for the Interactive Session**:

- **Per-process identity, not per-executable** — Each monitored app runs as a child process bound to a Windows **Job Object**, so two `node.exe` instances are tracked as separate entities. The Job Object also guarantees clean termination of the entire process tree — no zombies.
- **Live config reload** — Changes to `config.toml` are applied instantly, whether edited via the built-in UI or an external editor.
- **Minimal footprint** — The resident background process is pure Go, consumes a few megabytes of RAM, and is event-driven via `WaitForSingleObject` (zero idle CPU).

For deeper background, see [Design Rationales](./doc/rationales.md); for architecture and technical details, see [Design & Architecture](./doc/design.md).

## Installation

### Option A: Download from GitHub Releases (recommended)

1. Grab the latest `resurrector-<version>-windows-amd64.zip` from the [Releases page](../../releases).
2. Extract it anywhere you like (e.g. `%LOCALAPPDATA%\Programs\Resurrector\`).
3. Run `resurrector.exe`.

### Option B: Install via WinGet

```powershell
winget install --id Yanother.Resurrector
```

### Option C: Build from source

See [Building from source](#building-from-source) below.

## Quick Start

1. Run `resurrector.exe`. An icon appears in the system tray.
2. Right-click the tray icon and select **Open Settings** to launch the management UI.
3. Add the apps you want monitored and save. Resurrector starts watching them immediately.

## Usage

### Tray Menu

![Tray menu](https://github.com/user-attachments/assets/7bdcc088-7cee-40ac-b958-f22813615e30)

- **Resurrector v\<version\>**: A read-only header that shows the version of the currently running build. Useful when checking whether an update has been picked up.
- **Open Settings**: Launches the management UI. If the UI is already open, Resurrector does not launch a second copy.
- **Open config with...**: Opens `config.toml` in an editor of your choice (via the Windows "Open with" dialog). Useful when you want to edit the file directly instead of using the UI.
- **Auto-start Resurrector**: Toggles Windows sign-in auto-start for the current user by updating `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.
- **Quit**: Stops all monitored processes and exits Resurrector.

### Startup Behavior

- Only one instance of Resurrector may run **per Windows session** (not per machine). The single-instance guard uses a session-local named mutex, so different signed-in users can each run their own Resurrector independently. Launching a second copy within the same session shows an error dialog and exits.
- On first launch, if the config file does not exist, Resurrector creates it with sample content automatically.
- If the config file is invalid at startup, Resurrector shows an error dialog and exits.
- If the config file becomes invalid while Resurrector is already running, the current monitored state is kept and the change is ignored until the file is fixed.

## Configuration

### File Location

Applications to be monitored are managed via a `config.toml` file, located at `%USERPROFILE%\.config\resurrector\config.toml` by default. The file is automatically created with sample content on first launch. You can override the path with the `-f <path>` CLI flag.

> [!WARNING]
> Resurrector watches this file for real-time changes using `fsnotify`, so **atomic writes are required** when modifying it with external tools. The built-in UI handles this automatically. If using external scripts, write to a `.tmp` file and perform an atomic rename/move.

> [!TIP]
> While the TOML file requires arguments as a string array (`args = ["-a", "-b"]`), the management UI accepts them as a single shell-like string (e.g. `-a -b "quoted string"`) and parses them automatically.

### Example

```toml
# Resurrector Configuration

["PowerToys Awake"]
command = 'C:\Program Files\PowerToys\modules\Awake\PowerToys.Awake.exe'
args = ["--use-pt-config"]
cwd = 'C:\Program Files\PowerToys\modules\Awake'
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

### Field Reference

Each top-level TOML table (`[name]`) defines a single monitored app. `name` is the identifier shown in the UI.

| Field                 | Type         | Default                         | Description                                                                                                                                                                                                                                               |
| --------------------- | ------------ | ------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `command`             | String       | —                               | **Required.** Full path to the command or executable. If a relative path or bare command name (e.g. `npm`) is given, the absolute path is resolved via the system `PATH`.                                                                                 |
| `args`                | String array | `[]`                            | Arguments passed to `command`.                                                                                                                                                                                                                            |
| `cwd`                 | String       | Directory of resolved `command` | Working directory when launching the process.                                                                                                                                                                                                             |
| `enabled`             | Boolean      | `true`                          | Whether Resurrector launches and monitors this app. Set to `false` to keep the entry defined but paused (it will not be started automatically, and the UI shows it as disabled).                                                                          |
| `hide_window`         | Boolean      | `false`                         | If `true`, launches the process in the background (hidden window).                                                                                                                                                                                        |
| `restart_delay_sec`   | Integer      | `0`                             | Wait time (seconds) before attempting a restart after detecting termination.                                                                                                                                                                              |
| `max_retries`         | Integer      | `-1` (infinite)                 | Retry count limit for a crashing app. `0` disables retry; negative values mean infinite retry.                                                                                                                                                            |
| `healthy_timeout_sec` | Integer      | `0`                             | If the process runs for at least this many seconds before exiting, the restart counter resets to 0. `0` disables the reset (the counter increments on every exit).                                                                                        |
| `stop_command`        | String       | `""`                            | Optional shutdown executable. Resolved via system `PATH` if relative. See [Stop behavior](#stop-behavior).                                                                                                                                                |
| `stop_args`           | String array | `[]`                            | Arguments passed to `stop_command`. Not shell-parsed; invoke `cmd.exe` / `powershell.exe` explicitly if shell features are needed. Supports `${PID}` / `${NAME}` (see [Placeholders and environment variables](#placeholders-and-environment-variables)). |
| `stop_timeout_sec`    | Integer      | `5`                             | How long to wait for graceful shutdown before falling back to `TerminateProcess`.                                                                                                                                                                         |

## Advanced Usage

### CLI Options

| Flag                       | Description                                                                             |
| -------------------------- | --------------------------------------------------------------------------------------- |
| `-f <path>`                | Custom path to `config.toml`. Default: `%USERPROFILE%\.config\resurrector\config.toml`. |
| `-log-file <path>`         | Write logs to the specified file in append mode. Default: `stderr`.                     |
| `-log-format <text\|json>` | Log output format. Default: `text`.                                                     |

### Placeholders and environment variables

The `command`, `args`, `cwd`, `stop_command`, and `stop_args` fields all support placeholder expansion:

- `${NAME}` — Replaced with the value of the environment variable `NAME`. It is an error if `NAME` is not defined.
- `${PID}` — Replaced with the monitored process PID. **Only valid inside `stop_args`**; any other field containing `${PID}` is rejected as an invalid config.
- `$$` — Produces a literal `$` character. Use this if you need a `$` that is not part of a placeholder.

Example (TOML literal strings, single-quoted, are recommended on Windows so backslashes don't need escaping):

```toml
["My App"]
command = '${USERPROFILE}\bin\myapp.exe'
args = ['--config', '${APPDATA}\myapp\config.json']
stop_command = 'taskkill'
stop_args = ['/PID', '${PID}', '/T']
```

> [!TIP]
> If you prefer double-quoted strings, remember TOML treats them as basic strings where backslashes are escape characters — so Windows paths must be doubled, e.g. `"${USERPROFILE}\\bin\\myapp.exe"`.

Expansion is single-pass (expanded values are not re-scanned for placeholders), so the content of environment variables is always treated literally.

### Stop behavior

If `stop_command` is specified, Resurrector runs it first and waits up to `stop_timeout_sec` for the monitored process to exit.

```toml
["My App"]
command = "myapp.exe"
stop_command = "taskkill"
stop_args = ["/PID", "${PID}", "/T"]
stop_timeout_sec = 5
```

If `stop_command` is not specified, Resurrector chooses the best-effort graceful stop method automatically from runtime observation:

1. If the target PID owns a non-console top-level window (excluding both the classic console host and ConPTY pseudo-console windows), post `WM_CLOSE`.
2. Otherwise, send `CTRL_BREAK_EVENT` to the child's process group.
3. If the process is still alive after `stop_timeout_sec`, call `TerminateProcess`.

## Building from source

### Prerequisites

> **Supported OS:** Windows 10 version 1809 (October 2018 Update) or later, and Windows 11. See [Supported OS](./doc/design.md#supported-os) in the design doc for details.

- Go 1.26+
- Node.js 22+ (LTS recommended)
- pnpm (`npm install -g pnpm`, or see https://pnpm.io/installation)
- Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0` — match the version pinned in `ui/go.mod` and `.github/workflows/release.yml`)

### Build

From the repository root:

```bash
pnpm install
pnpm run build
```

Outputs in `build/`:

- `resurrector.exe` — Background process that launches, monitors, and auto-restarts your apps
- `resurrector-ui.exe` — Management UI launched on demand from the tray

## License

This project is licensed under the MIT License — see the [LICENSE](./LICENSE) file for details.

## Author

Daisuke (yet another) Maki
