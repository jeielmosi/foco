#!/bin/sh
set -u

PROJECT="${PROJECT:-$(pwd)}"
MODE="${MODE:-accelerated}"
BIN="${BIN:-bin/foco}"
BIN_PURE="${BIN_PURE:-bin/foco-pure}"
PREFIX="${PREFIX:-${HOME:-/tmp}/.local}"
BINDIR="${BINDIR:-$PREFIX/bin}"
DESTDIR="${DESTDIR:-}"
INSTALL="${INSTALL:-install}"
CGO_ENABLED="${CGO_ENABLED:-1}"
GO_TAGS="${GO_TAGS:-vectorscan}"
GO="${GO:-go}"
GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

log() {
	printf '%s\n' "[install] $*"
}

die() {
	log "$*"
	exit 1
}

case "$MODE" in
	pure)
		source_bin="$BIN_PURE"
		make_target="build-pure"
		;;
	*)
		source_bin="$BIN"
		make_target="build"
		;;
esac

rm -f "$PROJECT/$source_bin"
"${MAKE:-make}" -C "$PROJECT" \
	GO="$GO" GOTOOLCHAIN="$GOTOOLCHAIN" \
	CGO_ENABLED="$CGO_ENABLED" GO_TAGS="$GO_TAGS" \
	"$make_target" || die "build failed"

[ -f "$PROJECT/$source_bin" ] || die "missing $source_bin"
command -v "$INSTALL" >/dev/null 2>&1 || die "$INSTALL was not found"

# The accelerated binary also looks for libhs in ../lib/foco (see build.sh), so
# the installed foco keeps working if the build-time libhs is removed or the
# system library changes.
LIBDIR="$(dirname "$BINDIR")/lib/foco"

linked_libhs() {
	command -v ldd >/dev/null 2>&1 || return 1
	ldd "$1" 2>/dev/null | awk '$1 ~ /^libhs\.so/ && $3 ~ /^\// {print $1, $3}'
}

install_binary() {
	log "installing $1 to ${DESTDIR}${BINDIR}/foco"
	"$INSTALL" -d "${DESTDIR}${BINDIR}" || die "could not create ${DESTDIR}${BINDIR}"
	"$INSTALL" -m 0755 "$PROJECT/$1" "${DESTDIR}${BINDIR}/foco" || die "install failed"
}

install_libhs() {
	[ "$(uname -s)" = "Linux" ] || return 0
	libs="$(linked_libhs "$PROJECT/$source_bin")" || return 0
	[ -n "$libs" ] || return 0
	"$INSTALL" -d "${DESTDIR}${LIBDIR}" || die "could not create ${DESTDIR}${LIBDIR}"
	printf '%s\n' "$libs" | while read -r name path; do
		log "bundling $name from $path into ${DESTDIR}${LIBDIR}"
		"$INSTALL" -m 0644 "$path" "${DESTDIR}${LIBDIR}/$name" || exit 1
	done || die "could not bundle libhs"
}

install_binary "$source_bin"
install_libhs

# Without DESTDIR the installed binary must start; if its library cannot be
# loaded, install the pure-Go build instead of leaving a broken foco.
if [ -z "$DESTDIR" ] && [ "$MODE" != "pure" ] && ! "${BINDIR}/foco" --version >/dev/null 2>&1; then
	log "installed foco does not start; installing the pure-Go build instead"
	"${MAKE:-make}" -C "$PROJECT" GO="$GO" GOTOOLCHAIN="$GOTOOLCHAIN" build-pure || die "pure-Go build failed"
	install_binary "$BIN_PURE"
	"${BINDIR}/foco" --version >/dev/null 2>&1 || die "installed foco does not start"
fi
