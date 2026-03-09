#!/usr/bin/env bash
set -euo pipefail

# adkgobot uninstall script.

INSTALL_DIR="${adkgobot_INSTALL_DIR:-/usr/local/bin}"
BIN_PATH="${INSTALL_DIR}/adkgobot"
RUNTIME_DIR="${HOME}/.adkgobot"

log() {
  printf "[adkgobot-uninstall] %s\n" "$*"
}

die() {
  printf "[adkgobot-uninstall] ERROR: %s\n" "$*" >&2
  exit 1
}

have() {
  command -v "$1" >/dev/null 2>&1
}

SUDO=""
if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  if have sudo; then
    SUDO="sudo"
  else
    die "Need root or sudo to remove installed binary from $INSTALL_DIR"
  fi
fi

if [[ -x "$BIN_PATH" ]]; then
  log "Removing $BIN_PATH"
  $SUDO rm -f "$BIN_PATH"
else
  log "Binary not found at $BIN_PATH"
fi

if [[ -d "$RUNTIME_DIR" ]]; then
  log "Removing runtime data $RUNTIME_DIR"
  rm -rf "$RUNTIME_DIR"
fi

log "Uninstall complete"
