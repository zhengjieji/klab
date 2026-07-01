#!/usr/bin/env bash
# klab doctor — detect host readiness (read-only, no changes).
#
# Reports chip / OS / tooling / accelerated host / RAM / disk and prints an
# overall verdict. Exit 0 if ready (warnings allowed), non-zero if not ready.
# Safe to run anywhere; `scripts/setup.sh` calls it before and after configuring.
#
# This shell script is the pre-binary bootstrap probe (setup.sh runs it before
# the klab binary exists). Once built, `klab doctor` uses the Go verdict layer
# in internal/host, which is unit-tested; keep the rules here in sync with it.
set -euo pipefail

INSTANCE="${KLAB_LIMA_INSTANCE:-klab}"

if [[ -t 1 ]]; then
	C_RED=$'\033[31m'; C_GRN=$'\033[32m'; C_YEL=$'\033[33m'; C_DIM=$'\033[2m'; C_RST=$'\033[0m'
else
	C_RED=""; C_GRN=""; C_YEL=""; C_DIM=""; C_RST=""
fi

fails=0
warns=0
pass() { printf '  %s✓%s %s\n' "$C_GRN" "$C_RST" "$*"; }
warn() { printf '  %s!%s %s\n' "$C_YEL" "$C_RST" "$*"; warns=$((warns + 1)); }
fail() { printf '  %s✗%s %s\n' "$C_RED" "$C_RST" "$*"; fails=$((fails + 1)); }
note() { printf '    %s%s%s\n' "$C_DIM" "$*" "$C_RST"; }
have() { command -v "$1" >/dev/null 2>&1; }

section() { printf '\n%s\n' "$*"; }

check_tool() { # name, fix-hint
	if have "$1"; then
		pass "$1 ($("$1" --version 2>/dev/null | head -n1 || echo present))"
	else
		fail "$1 not found"
		[[ -n "${2:-}" ]] && note "$2"
	fi
}

doctor_lima_instance() {
	have limactl || return 0
	local status
	status="$(limactl list "$INSTANCE" --format '{{.Status}}' 2>/dev/null || true)"
	if [[ -z "$status" ]]; then
		warn "lima instance '$INSTANCE' not created — run ./scripts/setup.sh"
		return 0
	fi
	if [[ "$status" != "Running" ]]; then
		warn "lima instance '$INSTANCE' exists but is '$status' — limactl start $INSTANCE"
		return 0
	fi
	pass "lima instance '$INSTANCE' running"
	if limactl shell "$INSTANCE" -- test -e /dev/kvm >/dev/null 2>&1; then
		pass "/dev/kvm present inside '$INSTANCE'"
		if limactl shell "$INSTANCE" -- sh -c 'command -v kvm-ok >/dev/null 2>&1 && sudo kvm-ok' >/dev/null 2>&1; then
			pass "KVM acceleration usable (kvm-ok)"
		else
			warn "kvm-ok did not confirm acceleration (install cpu-checker / re-provision)"
		fi
	else
		fail "/dev/kvm missing inside '$INSTANCE' — needs Apple M3+/macOS 15+ and nestedVirtualization"
	fi
}

doctor_macos() {
	section "host (macOS)"
	local brand osver osmajor mem_gb gen
	brand="$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo unknown)"
	osver="$(sw_vers -productVersion 2>/dev/null || echo 0)"
	osmajor="${osver%%.*}"

	gen=""
	if [[ "$brand" =~ Apple\ M([0-9]+) ]]; then gen="${BASH_REMATCH[1]}"; fi
	if [[ -n "$gen" ]]; then
		if ((gen >= 3)); then
			pass "chip: $brand (M$gen — nested virtualization supported)"
		else
			warn "chip: $brand (M$gen — no nested virtualization; in-VM KVM / microVM limited)"
		fi
	elif [[ "$brand" == *Intel* ]]; then
		warn "chip: $brand (Intel Mac — arm64 guests emulated; x86 native via KVM)"
	else
		warn "chip: $brand (could not classify)"
	fi

	if ((osmajor >= 15)); then
		pass "macOS $osver (>= 15, nested virt available)"
	else
		fail "macOS $osver (need 15+ for nested virtualization)"
	fi

	mem_gb=$(( $(sysctl -n hw.memsize 2>/dev/null || echo 0) / 1073741824 ))
	if ((mem_gb >= 16)); then
		pass "RAM ${mem_gb} GiB"
	elif ((mem_gb >= 8)); then
		warn "RAM ${mem_gb} GiB (8 GiB: 1–2 VMs comfortable; clusters are a functional-only squeeze)"
	else
		fail "RAM ${mem_gb} GiB (< 8 GiB is very tight)"
	fi

	local avail
	avail="$(df -g "$HOME" 2>/dev/null | awk 'NR==2 {print $4}' || echo '?')"
	if [[ "$avail" == "?" ]]; then
		warn "could not read free disk for $HOME"
	elif ((avail >= 60)); then
		pass "free disk ${avail} GiB"
	else
		warn "free disk ${avail} GiB (kernel trees + rootfs want ~60 GiB)"
	fi

	section "tooling"
	if have brew; then pass "Homebrew"; else fail "Homebrew not found"; note "install: https://brew.sh — or ./scripts/setup.sh"; fi
	check_tool limactl "brew install lima — or ./scripts/setup.sh"
	check_tool go "brew install go — or ./scripts/setup.sh"

	section "accelerated host"
	doctor_lima_instance
}

doctor_linux() {
	section "host (Linux)"
	if [[ -e /dev/kvm ]]; then
		pass "/dev/kvm present"
		if have kvm-ok && sudo kvm-ok >/dev/null 2>&1; then pass "KVM acceleration usable"; else warn "kvm-ok did not confirm (install cpu-checker)"; fi
	else
		fail "/dev/kvm missing — enable KVM (or nested virt on a cloud VM)"
	fi
	section "tooling"
	check_tool go "install Go 1.22+"
	check_tool qemu-system-aarch64 "install qemu"
	check_tool clang "install clang/llvm for kernel cross-compile"
}

printf '%sklab doctor%s\n' "$C_DIM" "$C_RST"
case "$(uname -s)" in
	Darwin) doctor_macos ;;
	Linux) doctor_linux ;;
	*) fail "unsupported OS: $(uname -s)" ;;
esac

section "verdict"
if ((fails > 0)); then
	printf '  %sNOT READY%s — %d issue(s), %d warning(s). Run ./scripts/setup.sh\n' "$C_RED" "$C_RST" "$fails" "$warns"
	exit 1
elif ((warns > 0)); then
	printf '  %sREADY%s with %d warning(s)\n' "$C_YEL" "$C_RST" "$warns"
else
	printf '  %sREADY%s\n' "$C_GRN" "$C_RST"
fi
