#!/bin/sh
set -u

MODE="${1:-build}"
BIN="${BIN:-bin/foco}"
GO="${GO:-go}"
GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"
FOCO_CGO="${FOCO_CGO:-1}"
FOCO_TAGS="${FOCO_TAGS:-vectorscan}"

export GOTOOLCHAIN

log() {
	printf '%s\n' "[foco] $*"
}

have_libhs() {
	command -v pkg-config >/dev/null 2>&1 && pkg-config --exists libhs 2>/dev/null
}

add_brew_path() {
	if command -v brew >/dev/null 2>&1; then
		prefix="$(brew --prefix vectorscan 2>/dev/null || true)"
		if [ -n "$prefix" ]; then
			export PKG_CONFIG_PATH="$prefix/lib/pkgconfig:${PKG_CONFIG_PATH:-}"
			export CGO_LDFLAGS="${CGO_LDFLAGS:-} -Wl,-rpath,$prefix/lib"
		fi
	fi
}

run_root() {
	if [ "$(id -u)" = "0" ]; then
		"$@"
	elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
		sudo -n "$@"
	else
		return 127
	fi
}

install_macos() {
	if ! command -v brew >/dev/null 2>&1; then
		log "Homebrew not found; cannot install Vectorscan"
		return 1
	fi
	log "installing pkg-config and vectorscan with Homebrew"
	brew install pkg-config vectorscan || return 1
	add_brew_path
	have_libhs
}

install_linux() {
	if command -v apt-get >/dev/null 2>&1; then
		log "installing pkg-config and Hyperscan with apt-get"
		run_root apt-get install -y pkg-config libhyperscan-dev ||
			{ run_root apt-get update && run_root apt-get install -y pkg-config libhyperscan-dev; } ||
			return 1
	elif command -v dnf >/dev/null 2>&1; then
		log "installing pkgconf-pkg-config and vectorscan-devel with dnf"
		run_root dnf install -y pkgconf-pkg-config vectorscan-devel || return 1
	elif command -v pacman >/dev/null 2>&1; then
		log "installing pkgconf and hyperscan with pacman"
		run_root pacman -S --noconfirm pkgconf hyperscan || return 1
	elif command -v zypper >/dev/null 2>&1; then
		log "installing pkg-config and hyperscan-devel with zypper"
		run_root zypper install -y pkg-config hyperscan-devel || return 1
	elif command -v apk >/dev/null 2>&1; then
		log "installing pkgconf and hyperscan-dev with apk"
		run_root apk add --no-cache pkgconf hyperscan-dev || return 1
	else
		log "no supported package manager found"
		return 1
	fi
	have_libhs
}

# Without root, unpack the distribution's Vectorscan/Hyperscan packages into a
# user-local prefix and link against it with an rpath, so accelerated builds
# do not require sudo.
LOCAL_HS="${FOCO_VECTORSCAN_PREFIX:-${XDG_DATA_HOME:-$HOME/.local/share}/foco/vectorscan}"

add_local_path() {
	for dir in "$LOCAL_HS/usr/lib64" "$LOCAL_HS/usr/lib" "$LOCAL_HS"/usr/lib/*-linux-gnu; do
		if [ -f "$dir/pkgconfig/libhs.pc" ]; then
			export PKG_CONFIG_PATH="$dir/pkgconfig:${PKG_CONFIG_PATH:-}"
			export CGO_LDFLAGS="${CGO_LDFLAGS:-} -Wl,-rpath,$dir"
			return 0
		fi
	done
	return 1
}

fetch_local_linux() {
	work="$(mktemp -d)" || return 1
	if command -v dnf >/dev/null 2>&1 && command -v rpm2cpio >/dev/null 2>&1 && command -v cpio >/dev/null 2>&1; then
		log "downloading vectorscan-devel with dnf into $LOCAL_HS (no root)"
		(cd "$work" && dnf download -q vectorscan vectorscan-devel) || { rm -rf "$work"; return 1; }
		mkdir -p "$LOCAL_HS"
		for package in "$work"/*.rpm; do
			(cd "$LOCAL_HS" && rpm2cpio "$package" | cpio -idm --quiet) || { rm -rf "$work"; return 1; }
		done
	elif command -v apt-get >/dev/null 2>&1 && command -v dpkg-deb >/dev/null 2>&1; then
		log "downloading libhyperscan-dev with apt-get into $LOCAL_HS (no root)"
		runtime="$(apt-cache depends libhyperscan-dev 2>/dev/null | awk '/Depends: libhyperscan[0-9]/ {print $2; exit}')"
		(cd "$work" && apt-get download -q libhyperscan-dev $runtime) || { rm -rf "$work"; return 1; }
		mkdir -p "$LOCAL_HS"
		for package in "$work"/*.deb; do
			dpkg-deb -x "$package" "$LOCAL_HS" || { rm -rf "$work"; return 1; }
		done
	else
		rm -rf "$work"
		return 1
	fi
	rm -rf "$work"
	for pc in "$LOCAL_HS"/usr/lib64/pkgconfig/libhs.pc "$LOCAL_HS"/usr/lib/pkgconfig/libhs.pc "$LOCAL_HS"/usr/lib/*-linux-gnu/pkgconfig/libhs.pc; do
		[ -f "$pc" ] || continue
		sed -e "s#^prefix=.*#prefix=$LOCAL_HS/usr#" \
			-e 's#^exec_prefix=.*#exec_prefix=${prefix}#' \
			-e "s#^libdir=/usr/#libdir=\${prefix}/#" \
			-e "s#^includedir=/usr/#includedir=\${prefix}/#" \
			"$pc" > "$pc.tmp" && mv "$pc.tmp" "$pc"
	done
	add_local_path && have_libhs
}

ensure_vectorscan() {
	add_brew_path
	add_local_path
	have_libhs && return 0
	case "$(uname -s)" in
		Darwin)
			install_macos && return 0
			;;
		Linux)
			install_linux && return 0
			fetch_local_linux && return 0
			;;
	esac
	return 1
}

want_accelerated() {
	[ "$FOCO_CGO" = "1" ] || return 1
	case "$FOCO_TAGS" in
		*vectorscan*) return 0 ;;
	esac
	return 1
}

build_accelerated() {
	log "building with Hyperscan/Vectorscan"
	CGO_ENABLED=1 "$GO" build -tags "$FOCO_TAGS" -trimpath -o "$BIN" ./cmd/cli
}

build_pure() {
	log "building pure-Go fallback"
	CGO_ENABLED=0 "$GO" build -trimpath -o "$BIN" ./cmd/cli
}

case "$MODE" in
	check|--check)
		add_brew_path
		add_local_path
		if have_libhs; then
			log "libhs $(pkg-config --modversion libhs) found"
			exit 0
		fi
		log "libhs not found"
		exit 1
		;;
	deps|--deps-only)
		if ensure_vectorscan; then
			log "libhs $(pkg-config --modversion libhs) ready"
			exit 0
		fi
		log "could not install libhs"
		exit 1
		;;
	build|--build)
		mkdir -p "$(dirname "$BIN")"
		rm -f "$BIN"
		if want_accelerated; then
			if ensure_vectorscan; then
				if build_accelerated; then
					exit 0
				fi
				log "accelerated build failed; retrying pure-Go"
			else
				log "Vectorscan unavailable; using pure-Go fallback"
			fi
		else
			log "accelerated build disabled; using pure-Go fallback"
		fi
		build_pure
		;;
	*)
		log "unknown mode: $MODE"
		exit 2
		;;
esac
