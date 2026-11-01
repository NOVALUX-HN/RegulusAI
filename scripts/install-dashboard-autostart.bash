#!/usr/bin/env bash
# Regulus dashboard — autostart on POSIX (mirror of scripts/install-dashboard-autostart.ps1).
#
# Usage:
#   bash scripts/install-dashboard-autostart.bash [--port N] [--repo DIR] [--bind ADDR]
#   bash scripts/install-dashboard-autostart.bash --uninstall [--port N] [--repo DIR]
#
# What it does (same promise as the .ps1: fixed port, loopback-only, idempotent):
#   1. Resolves the repo root and the regulus binary (bin/regulus, else PATH).
#   2. Linux: writes a systemd user unit
#      ~/.config/systemd/system/regulus-dashboard.service and, when
#      `systemctl --user` exists, reloads + enables it. Without systemd it
#      prints a cron @reboot line to add manually.
#   3. macOS: writes ~/Library/LaunchAgents/ai.regulus.dashboard.plist and,
#      when `launchctl` exists, loads it.
#   4. Always prints the manual command, so the panel starts on any POSIX
#      box with or without a service manager: `regulus serve --port=17817`.
#
# Honest minimum: the script only claims what it verified — the binary
# answers `serve --help`, and the unit/plist file was written. Enabling the
# service is attempted, but a failure prints the manual fallback instead of
# claiming success. Pure bash + coreutils. Exit 0 on success, 1 otherwise.

set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
port="17817"
repo="$root"
bind="127.0.0.1"
uninstall="0"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --port=*) port="${1#--port=}"; shift ;;
    --repo=*) repo="${1#--repo=}"; shift ;;
    --root=*) repo="${1#--root=}"; shift ;;
    --bind=*) bind="${1#--bind=}"; shift ;;
    --uninstall) uninstall="1"; shift ;;
    --help|-h) sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "install-dashboard-autostart: unknown flag $1 (use --port=N --repo=DIR --bind=ADDR --uninstall)" >&2; exit 1 ;;
  esac
done

fail() {
  echo "install-dashboard-autostart: $1" >&2
  exit 1
}

case "$port" in
  ''|*[!0-9]*) fail "port must be an integer 1..65535, got '$port'." ;;
esac
[ "$port" -ge 1 ] && [ "$port" -le 65535 ] || fail "port must be an integer 1..65535, got '$port'."
[ -d "$repo" ] || fail "repo directory not found: $repo."
repo="$(cd "$repo" && pwd)"

# Binary: prefer the repo build, fall back to PATH (same precedence idea
# as the .ps1, which prefers bin\regulus.exe then bin\regulus).
bin="$repo/bin/regulus"
if [ ! -x "$bin" ]; then
  if command -v regulus >/dev/null 2>&1; then
    bin="$(command -v regulus)"
  else
    fail "no regulus binary: $repo/bin/regulus is not executable and regulus is not on PATH (build with: go build -o bin/regulus ./cmd/regulus)."
  fi
fi
"$bin" serve --help >/dev/null 2>&1 || fail "$bin does not answer 'serve --help'."

unit_name="regulus-dashboard"
os="$(uname -s)"

remove_linux() {
  unit="$HOME/.config/systemd/system/${unit_name}.service"
  if [ -f "$unit" ]; then
    rm -f "$unit"
    echo "removed: $unit"
  else
    echo "nothing to remove"
  fi
  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user daemon-reload >/dev/null 2>&1 || true
  fi
}

remove_darwin() {
  plist="$HOME/Library/LaunchAgents/ai.regulus.dashboard.plist"
  if command -v launchctl >/dev/null 2>&1; then
    launchctl unload -w "$plist" >/dev/null 2>&1 || true
  fi
  if [ -f "$plist" ]; then
    rm -f "$plist"
    echo "removed: $plist"
  else
    echo "nothing to remove"
  fi
}

if [ "$uninstall" = "1" ]; then
  case "$os" in
    Darwin) remove_darwin ;;
    *) remove_linux ;;
  esac
  exit 0
fi

manual="$bin serve --port=$port --bind=$bind (repo $repo)"

case "$os" in
  Darwin)
    dir="$HOME/Library/LaunchAgents"
    plist="$dir/ai.regulus.dashboard.plist"
    mkdir -p "$dir"
    cat >"$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>ai.regulus.dashboard</string>
  <key>ProgramArguments</key>
  <array>
    <string>$bin</string>
    <string>serve</string>
    <string>--port=$port</string>
    <string>--bind=$bind</string>
  </array>
  <key>WorkingDirectory</key><string>$repo</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><false/>
</dict>
</plist>
EOF
    echo "installed: $plist -> $manual"
    if command -v launchctl >/dev/null 2>&1; then
      if launchctl load -w "$plist" >/dev/null 2>&1; then
        echo "loaded with launchctl (starts at login)."
      else
        echo "note: launchctl load failed; start manually: $manual"
      fi
    else
      echo "note: launchctl not found; start manually: $manual"
    fi
    ;;
  *)
    dir="$HOME/.config/systemd/system"
    unit="$dir/${unit_name}.service"
    mkdir -p "$dir"
    cat >"$unit" <<EOF
[Unit]
Description=Regulus dashboard (read-only, loopback)
After=network.target

[Service]
ExecStart=$bin serve --port=$port --bind=$bind
WorkingDirectory=$repo
Restart=on-failure

[Install]
WantedBy=default.target
EOF
    echo "installed: $unit -> $manual"
    if command -v systemctl >/dev/null 2>&1; then
      systemctl --user daemon-reload >/dev/null 2>&1 || true
      if systemctl --user enable --now "$unit_name" >/dev/null 2>&1; then
        echo "enabled with systemctl --user (starts at login)."
      else
        echo "note: systemctl --user enable failed; start manually or add this cron line (crontab -e):"
        echo "@reboot $bin serve --port=$port --bind=$bind >>\$HOME/.regulus-dashboard.log 2>&1"
      fi
    else
      echo "note: no systemctl --user here; start manually or add this cron line (crontab -e):"
      echo "@reboot $bin serve --port=$port --bind=$bind >>\$HOME/.regulus-dashboard.log 2>&1"
    fi
    ;;
esac

echo "open http://localhost:$port/ after login (or run the manual command now)."
