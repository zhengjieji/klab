#!/usr/bin/env bash
# klab setup — detect the environment and auto-configure it.
#
# Does NOT assume anything is installed. On macOS it ensures Homebrew, lima, and
# Go are present, then starts (or reuses) the accelerated Lima host and verifies
# /dev/kvm. Idempotent — safe to re-run.
#
#   ./scripts/setup.sh            interactive (asks before installing)
#   ./scripts/setup.sh --yes      non-interactive (auto-confirm installs)
#   ./scripts/setup.sh --no-install   only configure; never install packages
set -euo pipefail

DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd -- "$DIR/.." && pwd)"
INSTANCE="${KLAB_LIMA_INSTANCE:-klab}"
LIMA_CONFIG="$DIR/lima/klab.yaml"

ASSUME_YES=0
NO_INSTALL=0
while [[ $# -gt 0 ]]; do
	case "$1" in
		--yes | -y) ASSUME_YES=1 ;;
		--no-install) NO_INSTALL=1 ;;
		-h | --help)
			grep -E '^#( |$)' "$0" | sed -E 's/^# ?//'
			exit 0
			;;
		*)
			echo "unknown option: $1" >&2
			exit 2
			;;
	esac
	shift
done

log() { printf '[setup] %s\n' "$*"; }
die() {
	printf '[setup][error] %s\n' "$*" >&2
	exit 1
}
have() { command -v "$1" >/dev/null 2>&1; }

confirm() { # prompt
	[[ "$ASSUME_YES" -eq 1 ]] && return 0
	local reply
	read -r -p "[setup] $1 [y/N] " reply || true
	[[ "$reply" =~ ^[Yy]$ ]]
}

ensure_brew() {
	have brew && return 0
	[[ "$NO_INSTALL" -eq 1 ]] && die "Homebrew missing and --no-install set. Install from https://brew.sh"
	confirm "Homebrew not found. Install it now?" ||
		die "Homebrew is required. Install from https://brew.sh, then re-run."
	log "installing Homebrew"
	NONINTERACTIVE=1 /bin/bash -c \
		"$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
	have brew || die "Homebrew install did not put brew on PATH; open a new shell and re-run."
}

ensure_brew_pkg() { # command, formula
	local cmd="$1" formula="$2"
	have "$cmd" && {
		log "$cmd present"
		return 0
	}
	[[ "$NO_INSTALL" -eq 1 ]] && die "$cmd missing and --no-install set (brew install $formula)"
	confirm "$cmd not found. brew install $formula?" || die "$cmd is required."
	log "brew install $formula"
	brew install "$formula"
}

start_host() {
	[[ -f "$LIMA_CONFIG" ]] || die "lima config not found: $LIMA_CONFIG"
	local status
	status="$(limactl list "$INSTANCE" --format '{{.Status}}' 2>/dev/null || true)"
	# First boot provisions the kernel toolchain over apt; give limactl generous
	# headroom so a slow first run does not trip its default readiness timeout.
	local start_flags=(--timeout "${KLAB_START_TIMEOUT:-20m}")
	if [[ "$status" == "Running" ]]; then
		log "lima instance '$INSTANCE' already running"
		return 0
	fi
	if [[ -n "$status" ]]; then
		log "starting existing lima instance '$INSTANCE' (was: $status)"
		limactl start "${start_flags[@]}" "$INSTANCE"
		return 0
	fi
	log "creating lima instance '$INSTANCE' from $LIMA_CONFIG (this provisions the toolchain; takes a few minutes)"
	local tty_flag=()
	[[ "$ASSUME_YES" -eq 1 ]] && tty_flag=(--tty=false)
	limactl start --name="$INSTANCE" "${tty_flag[@]}" "${start_flags[@]}" "$LIMA_CONFIG"
}

main() {
	case "$(uname -s)" in
		Darwin) ;;
		Linux)
			log "Linux host: ensure /dev/kvm and the kernel toolchain are present"
			exec "$DIR/doctor.sh"
			;;
		*) die "unsupported OS: $(uname -s)" ;;
	esac

	log "checking + configuring environment (instance: $INSTANCE)"
	ensure_brew
	ensure_brew_pkg limactl lima
	ensure_brew_pkg go go
	start_host

	log "verifying readiness"
	if "$DIR/doctor.sh"; then
		log "done. next: (cd '$ROOT' && make build) then ./bin/klab up examples/topologies/single.yaml"
	else
		die "host configured but doctor reports issues above; resolve them and re-run ./scripts/doctor.sh"
	fi
}

main "$@"
