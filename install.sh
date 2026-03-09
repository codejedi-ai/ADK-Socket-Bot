#!/usr/bin/env bash
set -euo pipefail

# Bootstrap installer for adkgobot.
# Intended usage:
#   curl -fsSL https://<your-domain>/install.sh | bash
#
# It installs prerequisites first, then invokes the official adkgobot installer.

OFFICIAL_URL_DEFAULT="https://raw.githubusercontent.com/codejedi-ai/adkgobot/main/scripts/install.sh"
OFFICIAL_URL="${adkgobot_OFFICIAL_INSTALL_URL:-$OFFICIAL_URL_DEFAULT}"

log() {
	printf "[adkgobot-bootstrap] %s\n" "$*"
}

warn() {
	printf "[adkgobot-bootstrap] WARNING: %s\n" "$*" >&2
}

die() {
	printf "[adkgobot-bootstrap] ERROR: %s\n" "$*" >&2
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
		die "This script needs root privileges to install packages. Install sudo or run as root."
	fi
fi

install_with_apt() {
	$SUDO apt-get update -y
	$SUDO DEBIAN_FRONTEND=noninteractive apt-get install -y \
		ca-certificates curl git bash tar gzip unzip build-essential golang
}

install_with_dnf() {
	$SUDO dnf install -y \
		ca-certificates curl git bash tar gzip unzip gcc make golang
}

install_with_yum() {
	$SUDO yum install -y \
		ca-certificates curl git bash tar gzip unzip gcc make golang
}

install_with_pacman() {
	$SUDO pacman -Sy --noconfirm \
		ca-certificates curl git bash tar gzip unzip base-devel go
}

install_with_zypper() {
	$SUDO zypper --non-interactive refresh
	$SUDO zypper --non-interactive install \
		ca-certificates curl git bash tar gzip unzip gcc make go
}

install_with_apk() {
	$SUDO apk add --no-cache \
		ca-certificates curl git bash tar gzip unzip build-base go
}

install_with_brew() {
	if ! have brew; then
		die "Homebrew is not installed. Install Homebrew first: https://brew.sh"
	fi
	brew update
	brew install curl git go || true
}

install_prereqs() {
	log "Installing prerequisites (curl, git, build tools, Go)"

	if have apt-get; then
		install_with_apt
	elif have dnf; then
		install_with_dnf
	elif have yum; then
		install_with_yum
	elif have pacman; then
		install_with_pacman
	elif have zypper; then
		install_with_zypper
	elif have apk; then
		install_with_apk
	elif [[ "$(uname -s)" == "Darwin" ]]; then
		install_with_brew
	else
		warn "No supported package manager detected. Checking required commands manually."
	fi

	local required=(bash curl git tar gzip)
	for cmd in "${required[@]}"; do
		have "$cmd" || die "Missing required command: $cmd"
	done

	if ! have go; then
		die "Go is not installed and could not be installed automatically. Please install Go 1.25+ and retry."
	fi

	log "Prerequisites ready"
}

run_official_install() {
	log "Running official adkgobot installer: $OFFICIAL_URL"
	tmp_script="$(mktemp)"
	trap 'rm -f "$tmp_script"' EXIT

	curl -fsSL "$OFFICIAL_URL" -o "$tmp_script"
	chmod +x "$tmp_script"
	bash "$tmp_script"
}

main() {
	install_prereqs
	run_official_install
	log "Install complete"
}

main "$@"
