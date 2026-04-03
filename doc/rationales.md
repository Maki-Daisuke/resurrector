# Design Rationales

This document records the reasoning behind key design decisions in Resurrector — the *why*, not just the *what*. For the architecture and implementation details, see [Design & Architecture](./design.md).

## Why Resurrector Exists

### The Problem: No Process-Level Monitor for Windows Interactive Sessions

On Linux, tools like `systemd` provide robust process supervision out of the box. On Windows, however, the **Interactive Session** (the desktop session where the user is logged in) has no equivalent built-in mechanism for monitoring and auto-restarting arbitrary processes.

The closest existing tool is [**"Restart on Crash"**](https://w-shadow.com/blog/2009/03/04/restart-on-crash/), but it takes a fundamentally different approach: it monitors whether a specific **executable file** (e.g. `notepad.exe`) is running. This works for simple cases but breaks down when the same executable hosts multiple unrelated applications.

**Example:** Consider two separate Node.js applications — a dev server and a build watcher. Both run as `node.exe`. "Restart on Crash" cannot distinguish between them because it only sees the executable name. If one crashes, it has no way of knowing *which* application needs restarting, or whether the other `node.exe` instance is the one it should be monitoring.

Resurrector solves this by owning the **entire process lifecycle**. It spawns each monitored application as a child process bound to a **Windows Job Object**, giving it a unique identity tied to the specific command, arguments, and working directory — not just the executable name. This means two `node.exe` instances with different arguments are tracked as completely separate entities.

## Why Core and UI Are Separate Executables

Resurrector is split into two independent binaries: the **core process** (`resurrector.exe`) and the **UI process** (`resurrector-ui.exe`). This is not an accident — it is a direct consequence of Resurrector's design goal: **do as little as possible, as efficiently as possible.**

Over its lifetime, Resurrector spends more than 99% of its time doing one thing: quietly watching processes via `WaitForSingleObject`. During this steady state, there is no user interaction, no rendering, no UI framework loaded in memory. The core is a pure Go binary with a minimal footprint — a few megabytes of RAM at most.

If the UI were bundled into the same process, the Wails runtime, the Svelte application, and the WebView2 renderer would all be loaded into memory permanently — even though the user might open the settings screen once a week (or never). That is a waste of resources for a tool whose entire purpose is to sit invisibly in the system tray.

By launching the UI as a **separate, disposable process**, Resurrector pays the cost of the UI only when the user explicitly requests it. When the settings window is closed, the UI process terminates and all of its memory is reclaimed by the OS. The core continues running undisturbed.

This separation also enforces a clean architectural boundary: the core **never writes** to `config.toml` and the UI **never manages processes**. Communication flows through the config file (UI → Core, via atomic writes and fsnotify) and through stdio-based IPC (Core → UI, for real-time status updates).

## Why We Do Not Inspect Exit Codes

### The Premise: Monitored Apps Are "Always-On"

Resurrector's purpose is to keep **always-on applications** running. These are processes that, by design, should never terminate on their own. A web server, a background daemon, a dev tool — if it exits, something went wrong, regardless of the exit code.

Under this premise:

- **Exit code 0 ("success")** does not mean "the app finished its job successfully." It means "the app stopped running, and it shouldn't have." A clean exit from a process that is supposed to run forever is just as much of a problem as a crash.
- **Exit code non-zero ("failure")** is the more obvious case, but the *action* is the same: restart.

### The Only Meaningful Distinction: Intentional vs. Unintentional

Rather than classifying exits by their code, Resurrector classifies them by **who caused the exit**:

| Exit cause | Action | How it's detected |
| --- | --- | --- |
| **Resurrector stopped the process** (user disabled it, config removed, app shutting down) | Do **not** restart | `stopChan` is closed before the process exits |
| **The process exited on its own** (crash, runtime error, unexpected clean exit, etc.) | **Restart** | `WaitForMultipleObjects` signals the process handle |

This is the only distinction that matters. Exit codes are an application-level concern and carry no universal meaning that a process supervisor can reliably act on.

### Comparison with Other Systems

- **systemd** offers `Restart=on-failure` (restart only on non-zero exit) and `Restart=always` (restart regardless of exit code). For always-on daemons, `Restart=always` is the standard recommendation — which is exactly what Resurrector does.
- **Windows SCM** (Service Control Manager) by default ignores exit codes entirely. Recovery actions trigger based on whether the service reported a failure to the SCM, not based on the exit code itself.

Resurrector's approach aligns with the `Restart=always` philosophy: **if it stopped, bring it back.**

## The Role of `healthy_timeout_sec` and `max_retries`

### Crash Loop Prevention, Not Exit Classification

Since every unintentional exit triggers a restart, there is a risk of an infinite crash loop: a misconfigured or broken application that starts, immediately crashes, restarts, immediately crashes, and so on forever.

The `healthy_timeout_sec` and `max_retries` parameters exist solely to address this:

- **`healthy_timeout_sec`**: Defines the minimum uptime (in seconds) for a run to be considered "stable." If the process runs for at least this long before exiting, the restart counter resets to 0. If it exits sooner, the counter increments.
- **`max_retries`**: The maximum number of consecutive "unstable" restarts before Resurrector gives up and marks the app as `Failed`. A negative value means infinite retries (never give up).

Together, they form a **crash loop breaker**: rapid repeated crashes are detected and eventually stopped, while a process that runs for a reasonable period and then crashes is given a fresh set of retries.

### Why Not Use Exit Codes for This?

One might argue that exit codes could help distinguish "real crashes" from "intentional stops." In practice, this adds complexity without meaningful benefit:

1. **No universal contract**: There is no standard that says exit code 0 means "I'm done, don't restart me." Many applications exit with 0 on unhandled signals or graceful shutdown paths that the user did not initiate.
2. **Runtime-specific behavior**: Different runtimes (Node.js, Python, Go, etc.) have different conventions for exit codes. A process supervisor cannot reliably interpret them without per-application configuration.
3. **Resurrector already knows**: If Resurrector itself requested the stop (via `stopChan` / Job Object termination), it already knows not to restart. No exit code inspection is needed.

The current design keeps the monitor simple, predictable, and runtime-agnostic.
