# Design Rationales

This document records the reasoning behind key design decisions in Resurrector — the _why_, not just the _what_. For the architecture and implementation details, see [Design & Architecture](./design.md).

## Why Resurrector Exists

### The Problem: No Process-Level Monitor for Windows Interactive Sessions

On Linux, tools like `systemd` provide robust process supervision out of the box. On Windows, however, the **Interactive Session** (the desktop session where the user is logged in) has no equivalent built-in mechanism for monitoring and auto-restarting arbitrary processes.

The closest existing tool is [**"Restart on Crash"**](https://w-shadow.com/blog/2009/03/04/restart-on-crash/), but it takes a fundamentally different approach: it monitors whether a specific **executable file** (e.g. `notepad.exe`) is running. This works for simple cases but breaks down when the same executable hosts multiple unrelated applications.

**Example:** Consider two separate Node.js applications — a dev server and a build watcher. Both run as `node.exe`. "Restart on Crash" cannot distinguish between them because it only sees the executable name. If one crashes, it has no way of knowing _which_ application needs restarting, or whether the other `node.exe` instance is the one it should be monitoring.

Resurrector solves this by owning the **entire process lifecycle**. It spawns each monitored application as a child process bound to a **Windows Job Object**, giving it a unique identity tied to the specific command, arguments, and working directory — not just the executable name. This means two `node.exe` instances with different arguments are tracked as completely separate entities.

## Why Core and UI Are Separate Executables

Resurrector is split into two independent binaries: the **core process** (`resurrector.exe`) and the **UI process** (`resurrector-ui.exe`). This is not an accident — it is a direct consequence of Resurrector's design goal: **do as little as possible, as efficiently as possible.**

Over its lifetime, Resurrector spends more than 99% of its time doing one thing: quietly watching processes via `WaitForSingleObject`. During this steady state, there is no user interaction, no rendering, no UI framework loaded in memory. The core is a pure Go binary with a minimal footprint — a few megabytes of RAM at most.

If the UI were bundled into the same process, the Wails runtime, the Svelte application, and the WebView2 renderer would all be loaded into memory permanently — even though the user might open the settings screen once a week (or never). That is a waste of resources for a tool whose entire purpose is to sit invisibly in the system tray.

By launching the UI as a **separate, disposable process**, Resurrector pays the cost of the UI only when the user explicitly requests it. When the settings window is closed, the UI process terminates and all of its memory is reclaimed by the OS. The core continues running undisturbed.

This separation also enforces a clean architectural boundary: after first-run bootstrap of a missing file, the core does not write to `config.toml`, and the UI never manages processes. Communication flows through the config file (UI → Core, via atomic writes and fsnotify) and through stdio-based IPC (Core → UI, for real-time status updates).

## Why Core and UI Communicate Over STDIO

The communication needs between the core and UI are intentionally narrow. The core launches the UI as its **child process**, pushes state updates while the window is open, and stops communicating when the window closes. This is not a general-purpose RPC system between independent long-lived services; it is a short-lived parent-child coordination channel.

For that reason, **stdio is a better fit than either named pipes or sockets**: unlike those mechanisms, it does not require publishing a separately reachable IPC endpoint, and it fits Resurrector's architecture well for three reasons:

- **Lifecycle coupling is automatic**: the channel exists only for that specific UI process instance.
- **Failure detection is simple**: if the core exits, the pipe closes and the UI can shut down immediately.
- **The protocol is one-way and small**: the UI only needs a stream of monitor state updates, not a rich bidirectional command channel.

Most importantly, this minimizes vulnerability surface. Resurrector does **not** publish a named pipe or socket that could be contacted unintentionally by external processes. By keeping communication inside the already-established parent-child stdio channel, it avoids introducing a separately discoverable IPC endpoint and reduces the chance of bugs turning into externally reachable attack paths.

Just as importantly, Resurrector does **not** want the UI-to-core control path to go through IPC at all. Configuration changes go through `config.toml`, which remains the Single Source of Truth. Using stdio only for transient status push keeps IPC narrow, keeps the core simple, and avoids creating a second state mutation interface alongside the config file.

## Why the Config File is TOML instead of XXX?

On Windows, many applications have historically used **INI-style configuration files**. That convention matters: a user-edited config file should feel immediately familiar instead of looking like an application-specific mini-language.

Resurrector uses **TOML** because it preserves much of that INI-like feel while fixing the limitations of INI itself. Table-based structure, `key = value` assignments, and overall visual simplicity make it approachable to users who are already accustomed to editing Windows config files by hand.

At the same time, TOML provides the expressiveness that plain INI lacks. Resurrector's config needs to represent booleans, integers, arrays such as `args`, and multiple named app entries in a way that is both structured and predictable. TOML supports that naturally without introducing much syntactic noise.

Just as importantly, TOML avoids the ambiguity that comes with looser configuration formats. Resurrector treats `config.toml` as a declarative source of truth, so the format should be easy for humans to read while remaining straightforward for Go code to parse into a strictly typed schema. In that sense, TOML was chosen as a practical successor to the traditional Windows INI style: familiar in shape, but with enough structure and clarity for a modern config file.

## Why `command` and `stop_command` Are Split Into an Executable and an Args Array

Both the main launch command and the optional shutdown command in `config.toml` are expressed as a **single executable string plus an array of arguments**:

```toml
command = "node.exe"
args = ["server.js", "--port", "8080"]

stop_command = "taskkill"
stop_args = ["/PID", "{pid}", "/T"]
```

An obvious alternative would be to pack everything into a single string — `command = "node.exe server.js --port 8080"` — the way `systemd`'s `ExecStart=` or a shell prompt does. Resurrector deliberately does not do this, for one overriding reason: **a single-string form invites users to believe they can write shell syntax in it.**

### Shell Syntax Does Not Work Here

Resurrector does not hand the command line to a shell at all. It constructs the process directly from an executable and an argv, without any intermediate shell interpretation. That means things like the following **will not do what the user expects**:

- `myapp.exe > output.log` — the `>` is passed as a literal argument to `myapp.exe`, not interpreted as a redirection. No file is opened, nothing is redirected.
- `myapp.exe | tee log.txt` — the `|` is a literal argument. There is no pipe.
- `myapp.exe && cleanup.exe` — `&&` is a literal argument. The second command is never run.
- `myapp.exe "$USER"` — environment variable expansion does not happen. The literal string `$USER` is passed through.

These characters are shell features, not process-creation features. If the config format accepted a single string, users would reasonably assume that shell features work there (because they work in every shell prompt, `.bat` file, and `ExecStart=` line they have seen). The result would be silent misconfiguration: the process starts, but not in the way the user intended.

### Why Not Just Pick a Shell and Interpret the String?

One way to make a single-string form "work" would be for Resurrector to pass the command to a shell internally. But on Windows, **there is no single obvious choice of shell**, and every option has significant drawbacks:

- **`cmd.exe`** is the traditional Windows shell. It supports `>`, `|`, `&&`, `%VAR%`, but its quoting and escaping rules are famously idiosyncratic, and it cannot express many things that modern users expect.
- **PowerShell** is the modern Windows shell. It supports `>`, `|`, `$env:VAR`, but its syntax is fundamentally different from `cmd.exe` — it has its own quoting rules, its own redirection semantics (e.g. `2>&1` works differently), and its own built-in aliases that do not exist in `cmd.exe`.
- **Bash / sh** may or may not be present, depending on whether Git for Windows, WSL, or MSYS2 is installed.

If Resurrector silently chose one of these, users writing commands for another shell would be surprised. A string like `myapp.exe 2>&1 | Tee-Object log.txt` is valid PowerShell but nonsense to `cmd.exe`; `myapp.exe && echo done` works in both `cmd.exe` and `bash` but with subtly different semantics around exit codes. There is no correct default.

The cleanest way to avoid this entire class of ambiguity is to **not interpret the command as shell syntax at all**. Resurrector sidesteps the question of "which shell?" by declaring, at the config level, that the command is not shell input. That removes an entire dimension of complexity — no shell selection, no quoting rules to document, no cross-shell portability concerns.

### Why an Array Avoids the Illusion

By requiring the arguments to be written as an explicit array, the config format makes it visually obvious that each element is a **separate argv entry**, not a shell command line. There is no place to put a redirection — `>` in an array element is just a string. The format itself tells the user, "this is argv, not shell input."

If the user genuinely needs shell features (pipes, redirection, conditional execution, variable expansion), the escape hatch is explicit and unambiguous — and, crucially, the user chooses which shell to invoke:

```toml
# Use cmd.exe explicitly
command = "cmd.exe"
args = ["/c", "myapp.exe > output.log"]

# Or PowerShell explicitly
command = "powershell.exe"
args = ["-Command", "myapp.exe 2>&1 | Tee-Object log.txt"]
```

Here, the user is clearly invoking a specific shell and handing it a command line to interpret. There is no ambiguity about who is parsing what, or which dialect applies.

### Consistency With the Rest of the Ecosystem

This design also matches the convention used by most modern process-management tools — Docker (`ENTRYPOINT` / `CMD`), Kubernetes (`command` / `args`), VS Code `tasks.json` (`command` / `args`), and PM2 (`script` / `args`) all separate the executable from its arguments, for essentially the same reason. Tools that fold everything into a single string (`systemd`, `supervisord`) do so for historical continuity with shell-script-based init systems, and they pay for it with a custom parser whose quoting and escaping rules are a frequent source of bugs.

Resurrector chooses the argv-separated style because it is **safer, more explicit, and free of shell illusions** — and it applies the same rule to both `command` and `stop_command` so the config schema stays consistent.

## Why We Do Not Inspect Exit Codes

### The Premise: Monitored Apps Are "Always-On"

Resurrector's purpose is to keep **always-on applications** running. These are processes that, by design, should never terminate on their own. A web server, a background daemon, a dev tool — if it exits, something went wrong, regardless of the exit code.

Under this premise:

- **Exit code 0 ("success")** does not mean "the app finished its job successfully." It means "the app stopped running, and it shouldn't have." A clean exit from a process that is supposed to run forever is just as much of a problem as a crash.
- **Exit code non-zero ("failure")** is the more obvious case, but the _action_ is the same: restart.

### The Only Meaningful Distinction: Intentional vs. Unintentional

Rather than classifying exits by their code, Resurrector classifies them by **who caused the exit**:

| Exit cause                                                                                | Action             | How it's detected                                   |
| ----------------------------------------------------------------------------------------- | ------------------ | --------------------------------------------------- |
| **Resurrector stopped the process** (user disabled it, config removed, app shutting down) | Do **not** restart | `stopChan` is closed before the process exits       |
| **The process exited on its own** (crash, runtime error, unexpected clean exit, etc.)     | **Restart**        | `WaitForMultipleObjects` signals the process handle |

This is the only distinction that matters. Exit codes are an application-level concern and carry no universal meaning that a process supervisor can reliably act on.

### Comparison with Other Systems

- **systemd** offers `Restart=on-failure` (restart only on non-zero exit) and `Restart=always` (restart regardless of exit code). For always-on daemons, `Restart=always` is the standard recommendation — which is exactly what Resurrector does.
- **Windows SCM** (Service Control Manager) by default ignores exit codes entirely. Recovery actions trigger based on whether the service reported a failure to the SCM, not based on the exit code itself.

Resurrector's approach aligns with the `Restart=always` philosophy: **if it stopped, bring it back.**

## The Role of `healthy_timeout_sec` and `max_retries`

### Crash Loop Prevention, Not Exit Classification

Since every unintentional exit triggers a restart, there is a risk of an infinite crash loop: a misconfigured or broken application that starts, immediately crashes, restarts, immediately crashes, and so on forever.

The `healthy_timeout_sec` and `max_retries` parameters exist solely to address this:

- **`healthy_timeout_sec`**: Defines the minimum uptime (in seconds) for a run to be considered "stable." If the process runs for at least this long before exiting, the restart counter resets to 0. If it exits sooner, the counter increments. A value of `0` disables the uptime-based reset: the counter increments on every exit so `max_retries` remains meaningful with the defaults.
- **`max_retries`**: The maximum number of consecutive "unstable" restarts before Resurrector gives up and marks the app as `Failed`. A negative value means infinite retries (never give up).

Together, they form a **crash loop breaker**: rapid repeated crashes are detected and eventually stopped, while a process that runs for a reasonable period and then crashes is given a fresh set of retries.

### Why Not Use Exit Codes for This?

One might argue that exit codes could help distinguish "real crashes" from "intentional stops." In practice, this adds complexity without meaningful benefit:

1. **No universal contract**: There is no standard that says exit code 0 means "I'm done, don't restart me." Many applications exit with 0 on unhandled signals or graceful shutdown paths that the user did not initiate.
2. **Runtime-specific behavior**: Different runtimes (Node.js, Python, Go, etc.) have different conventions for exit codes. A process supervisor cannot reliably interpret them without per-application configuration.
3. **Resurrector already knows**: If Resurrector itself requested the stop (via `stopChan` / Job Object termination), it already knows not to restart. No exit code inspection is needed.

The current design keeps the monitor simple, predictable, and runtime-agnostic.

## Why Graceful Stop Prefers WM_CLOSE and CTRL_BREAK_EVENT over TerminateProcess

When Resurrector needs to stop a monitored process (because the user disabled it, the entry was removed from `config.toml`, the config was modified in a way that requires a restart, or the core is shutting down), the ultimate fallback is always `TerminateProcess`. But Resurrector does not jump to it immediately. Instead, it first tries `WM_CLOSE` for processes that own a top-level window, and `CTRL_BREAK_EVENT` for console-attached processes, waiting up to `stop_timeout_sec` before escalating.

The reason is simple: **`TerminateProcess` gives the target no opportunity to clean up.**

`TerminateProcess` is a hard kill. The OS unmaps the process immediately, without running any user-mode code in the target. This means:

- **Open files are not flushed**. Buffered writes held in the process's userspace (language runtime buffers, stdio buffers, application-level caches) are lost. Even files that the OS will eventually close can be left in an inconsistent on-disk state if the application was mid-write.
- **`defer` / `atexit` / destructors do not run**. Language-level cleanup hooks — `defer` in Go, `finally` / `atexit` in Python, destructors in C++, `SIGTERM` handlers in Node.js — are all bypassed. Any invariant the application maintains through those hooks is violated.
- **Child processes and temp files may be orphaned**. The Job Object guarantees descendant processes are killed, but temp files, lock files, named pipes, and similar artifacts on disk that a well-behaved shutdown would remove are left behind.
- **Databases and stateful services may corrupt**. Embedded databases (SQLite, LevelDB, etc.) often rely on a shutdown sequence to checkpoint or release locks. A hard kill can leave them requiring recovery on next start, or in the worst case, corrupt.

`WM_CLOSE` and `CTRL_BREAK_EVENT` are each the idiomatic "please shut down" signal for their respective kinds of Windows programs:

- **`WM_CLOSE`** is what the OS sends when the user clicks the window's close button. GUI applications typically handle it by running their normal shutdown path — saving state, confirming unsaved changes (or, in well-behaved always-on apps, silently exiting cleanly).
- **`CTRL_BREAK_EVENT`** is the closest Windows equivalent to a Unix `SIGTERM` for console programs. Runtimes like Node.js, Go, and Python translate it into a signal/event their application code can catch and respond to.

Neither is guaranteed to succeed — an application may ignore `WM_CLOSE`, or a console app may not install a control handler. That is exactly why `stop_timeout_sec` exists: Resurrector asks nicely, gives the application a bounded window to comply, and then escalates to `TerminateProcess` only if the graceful path fails. The design goal is **graceful first, forceful fallback** — respecting the application's cleanup path whenever possible, while still guaranteeing that desired state eventually converges.
