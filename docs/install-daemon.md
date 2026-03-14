# Install baked daemon (systemd / launchd)

The **baked** daemon runs in the background, watches your Bakefile (and imports), updates shims when the config changes, and can run targets, suites, and **bake up** / **bake down** when the **bake** CLI connects to it. By default **bake** autostarts **baked** when needed; you can also run **baked** manually or register it with the OS so it starts on login or boot.

## Automated install (recommended)

From the repo root (directory containing the Bakefile):

```bash
bake install daemon
```

This detects the OS and:

- **Linux:** Writes a systemd user unit under `~/.config/systemd/user/` (e.g. `baked-<id>.service`), runs `systemctl --user daemon-reload`, then enables and starts the service so **baked** runs for this workspace and restarts on failure. The unit uses the workspace root as `WorkingDirectory` and the **baked** binary from your PATH (or next to **bake**).
- **macOS:** Writes a launchd plist under `~/Library/LaunchAgents/` (e.g. `com.bake.baked.<id>.plist`), then runs `launchctl load` so **baked** starts immediately and on login. **KeepAlive** restarts it if it exits.

You need **baked** on your PATH (or in the same directory as **bake**) before running **bake install daemon**. Each workspace gets its own unit/plist (identified by a short hash of the workspace path).

**Stop the installed daemon:**

- **Linux:** `systemctl --user stop baked-<id>.service` (the exact name is printed by **bake install daemon**).
- **macOS:** `launchctl unload ~/Library/LaunchAgents/com.bake.baked.<id>.plist`.

## Manual run

From the repo root:

```bash
baked
```

Flags:

- **`--no-watch`** — Disable file watching (for tests or when you only want the socket for run requests).
- **`--tcp <addr>`** — Also listen on TCP (e.g. `--tcp localhost:9876`) in addition to the Unix socket. Useful for remote or containerized clients.
- **`--workspace <dir>`** — Add additional workspace directories (repeatable). Each workspace gets its own config, watcher, and queue.

**baked** exits on SIGINT, SIGTERM, or SIGHUP. Send SIGUSR1 to force a config reload across all workspaces, or SIGUSR2 to dump status to the log.

## Manual systemd setup (fallback)

If you prefer a template unit that you enable per workspace:

**`~/.config/systemd/user/baked@.service`:**

```ini
[Unit]
Description=Baked daemon for %i
After=network.target

[Service]
Type=simple
WorkingDirectory=%i
ExecStart=/usr/bin/baked
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
```

Enable and start for a specific workspace:

```bash
systemctl --user enable baked@/path/to/your/repo.service
systemctl --user start baked@/path/to/your/repo.service
```

Use **`/usr/bin/baked`** or the path where **baked** is installed (e.g. **`$HOME/go/bin/baked`**).

## Manual launchd setup (fallback)

Create a plist with your repo root and **baked** path. Example **`~/Library/LaunchAgents/com.bake.baked.plist`:**

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.bake.baked</string>
  <key>ProgramArguments</key>
  <array>
    <string>/path/to/baked</string>
  </array>
  <key>WorkingDirectory</key>
  <string>/path/to/your/repo</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
</dict>
</plist>
```

Load and start:

```bash
launchctl load ~/Library/LaunchAgents/com.bake.baked.plist
```

Unload to stop:

```bash
launchctl unload ~/Library/LaunchAgents/com.bake.baked.plist
```

## Socket, state, and queue

- **Socket:** `.bake/baked.sock` in the workspace root. The **bake** CLI connects here when it delegates a run.
- **PID file:** `.bake/baked.pid` (optional; used by some setups to check if baked is running).
- **State:** **bake up** / **bake down** use `.bake/state.json`; when you run **bake up** via the daemon, the daemon owns that state.
- **Queue:** `.bake/queue.json` — Persistent FIFO queue. Incoming requests are written before execution and removed on completion. On restart, any pending items are replayed.

## Health checker

**baked** runs a periodic health check (every 30 seconds) that validates all daemon PIDs and containers in state. Dead entries are automatically removed. If the interval between checks exceeds 90 seconds (3× normal), baked infers the system slept and runs a full re-check.

## Hot reload

When the Bakefile changes (detected by the file watcher), **baked** diffs the old and new `up` target daemons:

- **Added** daemons are started automatically.
- **Removed** daemons are stopped and cleaned from state.
- **Changed** daemons (different image or steps) are restarted (stop old, start new).

No manual `bake down` / `bake up` cycle needed for daemon changes.

## See also

- [CLI reference](cli-reference.md) — **`--no-daemon`**, **`BAKE_DAEMON`**, **`BAKE_NO_DAEMON`**, and the Baked daemon section.
