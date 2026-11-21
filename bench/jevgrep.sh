#!/bin/sh
# Runs jevgrep (jg) as foco-bench's base: foco-bench run -base-bin bench/jevgrep.sh
# JG points at the jg executable (default: jg on PATH). The first call saves
# credentials for the fake provider in the bench's config home; every search
# runs with --no-cache, since foco has no response cache.
set -eu
JG="${JG:-jg}"
marker="$FOCO_BENCH_SCRATCH/jevgrep-auth"
if [ ! -f "$marker" ]; then
	printf 'foco-bench' | "$JG" auth --provider custom --base-url "$FOCO_BENCH_ENDPOINT" --model foco-bench --stdin >/dev/null
	: > "$marker"
fi
exec "$JG" --no-cache "$@"
