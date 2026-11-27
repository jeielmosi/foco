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

config_home="${XDG_CONFIG_HOME:-}"
if [ -z "$config_home" ]; then
	[ -n "${HOME:-}" ] || die "HOME is not set"
	config_home="$HOME/.config"
fi

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
[ -f "$PROJECT/SKILL.md" ] || die "missing $PROJECT/SKILL.md"

log "installing $source_bin to ${DESTDIR}${BINDIR}/foco"
"$INSTALL" -d "${DESTDIR}${BINDIR}" || die "could not create ${DESTDIR}${BINDIR}"
"$INSTALL" -m 0755 "$PROJECT/$source_bin" "${DESTDIR}${BINDIR}/foco" || die "install failed"

skill_dir="${DESTDIR}${config_home}/foco"
"$INSTALL" -d "$skill_dir" || die "could not create $skill_dir"
"$INSTALL" -m 0644 "$PROJECT/SKILL.md" "$skill_dir/SKILL.md" || die "skill install failed"
