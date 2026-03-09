#!/usr/bin/env bash
set -euo pipefail

# Official adkgobot installer.
# Installs adkgobot to /usr/local/bin by default.

REPO_DEFAULT="https://github.com/codejedi-ai/adkgobot.git"
REPO_URL="${adkgobot_REPO_URL:-$REPO_DEFAULT}"
INSTALL_DIR="${adkgobot_INSTALL_DIR:-/usr/local/bin}"
WORK_DIR="${adkgobot_WORK_DIR:-$(mktemp -d)}"

log() {
  printf "[adkgobot-install] %s\n" "$*"
}

die() {
  printf "[adkgobot-install] ERROR: %s\n" "$*" >&2
  exit 1
}

have() {
  command -v "$1" >/dev/null 2>&1
}

SUDO=""
if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  if have sudo; then
    SUDO="sudo"
  fi
fi

cleanup() {
  if [[ -n "${WORK_DIR:-}" && -d "${WORK_DIR:-}" ]]; then
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT

have git || die "git is required"
have go || die "go is required (Go 1.25+)"

log "Cloning source from $REPO_URL"
git clone --depth 1 "$REPO_URL" "$WORK_DIR/repo"

log "Building adkgobot"
(
  cd "$WORK_DIR/repo"
  go mod tidy
  go build -o adkgobot ./cmd/adkgobot
)

log "Installing adkgobot to $INSTALL_DIR"
$SUDO mkdir -p "$INSTALL_DIR"
$SUDO install -m 0755 "$WORK_DIR/repo/adkgobot" "$INSTALL_DIR/adkgobot"

if have adkgobot; then
  log "adkgobot installed: $(command -v adkgobot)"
else
  log "adkgobot installed to $INSTALL_DIR/adkgobot"
  log "If your PATH does not include $INSTALL_DIR, add it and reopen your shell."
fi

log "Next steps:"
log "1) adkgobot onboard"
log "2) adkgobot gateway start"
log "3) adkgobot tui"
