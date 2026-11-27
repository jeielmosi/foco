#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

project="$tmp/project"
home="$tmp/home"
mkdir -p "$project/scripts" "$home"
cp "$repo/scripts/install.sh" "$project/scripts/install.sh"
cp "$repo/SKILL.md" "$project/SKILL.md"

cat > "$tmp/fake-make" <<'SH'
#!/bin/sh
set -eu
mkdir -p "$FAKE_PROJECT/bin"
: > "$FAKE_PROJECT/bin/foco"
: > "$FAKE_PROJECT/bin/foco-pure"
SH
chmod +x "$tmp/fake-make"

run_install() {
	mode=$1
	xdg=$2
	destdir=$3
	HOME="$home" XDG_CONFIG_HOME="$xdg" PROJECT="$project" MODE="$mode" \
		BIN=bin/foco BIN_PURE=bin/foco-pure PREFIX="$tmp/prefix" \
		DESTDIR="$destdir" INSTALL=install MAKE="$tmp/fake-make" \
		FAKE_PROJECT="$project" sh "$project/scripts/install.sh"
}

assert_skill_mode() {
	mode=$(LC_ALL=C ls -ld "$1" | cut -c1-10)
	[ "$mode" = "-rw-r--r--" ] || {
		printf 'unexpected mode for %s: %s\n' "$1" "$mode" >&2
		exit 1
	}
}

run_install accelerated "" ""
cmp "$project/SKILL.md" "$home/.config/foco/SKILL.md"
assert_skill_mode "$home/.config/foco/SKILL.md"

xdg="$tmp/xdg"
destdir="$tmp/stage"
run_install pure "$xdg" "$destdir"
cmp "$project/SKILL.md" "$destdir$xdg/foco/SKILL.md"
assert_skill_mode "$destdir$xdg/foco/SKILL.md"
